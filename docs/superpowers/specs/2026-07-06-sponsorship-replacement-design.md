# Sponsorship Replacement — Design

**Date:** 2026-07-06
**Status:** Approved pending review

## Problem

A sponsor funds a specific student for a date range (`SponsorshipRecord` on the student
aggregate). Students can leave the system (status → INACTIVE) before the sponsorship's
`end_date`. Today nothing happens to the sponsorship: the sponsor keeps a "current"
sponsorship pointing at a departed student until the end date passes, and the money
effectively feeds no one. Admins need a way to replace the sponsored student with another
student for the remaining time.

## Decision summary

- **Semantics:** Truncate + transfer. The departed student's record is shortened to the
  replacement date; the replacement student receives a new sponsorship from the
  replacement date through the original `end_date`. Both records point at each other and
  are timestamped. History stays accurate for both students.
- **Event shape:** One admin command, **two events, one per aggregate** (there is no
  sponsor/sponsorship aggregate; sponsorships live inside each student's aggregate, and a
  single cross-aggregate event would break replay for the second student).
- **Reason:** The admin must supply a reason for the replacement; it is stored on the
  truncate event and surfaced in both students' history displays.
- **Replacement pool:** Same rules as the sponsor-facing flow — active,
  `eligible_for_sponsorship = true`, and no current or future sponsorship
  (`max_sponsorship_date` passed or null). Replacement must not be the same student.
- **Replacement date:** Today (server date), not admin-chosen.
- **Entry points:** Both the sponsored-students admin report and the student admin page.

## Domain layer (`events/student.proto`, `internal/student/aggregate.go`)

### New command/event: `Student_TruncateSponsorship`

Fields:

- `payment_id` — primary key to locate the target `SponsorshipRecord`; when empty, fall
  back to matching `sponsor_id` + `start_date`.
- `sponsor_id`, `start_date` — fallback identity of the record.
- `new_end_date` — the new end date; the aggregate only requires it to be ≥ the record's
  `start_date`. The "must shorten" rule (new end earlier than current end) is enforced by
  the service-level replacement flow, not the event handler — the compensation path (see
  Service layer) reuses this same event to restore the original, later end date.
- `replaced_by_student_id` — audit pointer to the replacement student.
- `reason` — required free-text reason (admin UI offers presets + free text).
- `version`, `metadata` — standard command envelope (metadata carries the timestamp).

Aggregate behavior:

- `TruncateSponsorship(cmd)` validates the record exists and that
  `new_end_date ≥ start_date`, then emits `EVENT_TRUNCATE_SPONSORSHIP`.
- `handleTruncateSponsorship(evt)` finds the matching record in `SponsorshipHistory` and
  sets its `end_date = new_end_date`, and stores `replaced_by_student_id` + `reason` on
  the record.

### Extended: `Student_UpdateSponsorship`

- Add `replaced_student_id` to the command, event, and `SponsorshipRecord` — back-pointer
  from the transferred sponsorship to the departed student. Empty for ordinary
  sponsorships.
- `SponsorshipRecord` gains `replaced_by_student_id`, `replaced_student_id`, and `reason`
  fields so history renders the full story on both sides.

### Bug fix (in scope)

`payment_id` / `payment_amount` are accepted by the API but silently dropped:
`UpdateSponsorship` omits them from the event (`aggregate.go:714`),
`handleUpdateSponsorship` omits them from the record (`aggregate.go:574`), and the
projection INSERT omits the columns (`repository.go:1494`). Fix the full chain so payment
info persists — the transfer needs to carry it over.

## Service layer (`internal/student/service.go`)

`ReplaceSponsorship(ctx, oldStudentID, newStudentID, sponsorshipKey, reason)`:

1. Load the old student; locate the target sponsorship by `sponsorshipKey`
   (`payment_id`, or `sponsor_id` + `start_date`). Require `end_date` strictly after
   today (a sponsorship ending today or earlier is not replaceable).
2. Validate the replacement student: exists, active, `eligible_for_sponsorship`, no
   current/future sponsorship, and `newStudentID != oldStudentID`.
3. Compute the window: truncate old record to today; new record runs today → original
   `end_date`, same `sponsor_id`, `payment_id`, `payment_amount` carried over.
4. Execute `TruncateSponsorship` on the old aggregate, then `UpdateSponsorship` (with
   `replaced_student_id`) on the new aggregate. There is no cross-aggregate transaction:
   if the second command fails, compensate by emitting a second
   `TruncateSponsorship` event on the old student with `new_end_date` set back to the
   original end date (the event is an end-date adjustment; only the admin-initiated flow
   requires shortening), then return the error.
   If compensation itself fails, log loudly with both student IDs so an operator can
   repair manually.

Projections flow through the existing `upsertSponsorshipProjections` on both students;
`max_sponsorship_date` recalculates naturally — the departed student's slot frees up and
the replacement student becomes unavailable.

## Admin web layer (`internal/webapi`)

- `GET /admin/sponsorship/replace` — picker page: given old student + sponsorship key,
  lists eligible + available students (existing list query with
  `EligibleForSponsorshipOnly` + availability filter; search by name/school).
- `POST /admin/sponsorship/replace` — executes the replacement. Form fields: old student
  ID, sponsorship key, new student ID, reason (required; presets "Student left school",
  "Moved away", "Graduated", plus free text).
- Confirmation step shows: sponsor, old student, new student, remaining window
  (today → end_date), and reason before submit.
- **Entry point 1 — sponsored-students report** (`sponsored_students.templ`): "Replace
  student" button per sponsorship row.
- **Entry point 2 — student admin page** (`admin_view_student.templ`): same button next
  to any current sponsorship.
- Display: truncated records render "Replaced by <new student> on <date> — <reason>";
  transferred records render "Continued from <old student> on <date>".

## Edge cases

- Sponsorship already ended (`end_date` ≤ today): refuse with a clear error.
- Replacement student equals old student: refuse.
- Replacement student became ineligible/sponsored between picker and submit: command
  validation re-checks at execution time and refuses.
- Second command failure: compensation restores old record; error surfaced to admin.

## Testing

- Aggregate: truncate happy path; record not found; record already ended;
  `new_end_date` before `start_date`; reason/pointers stored on both record shapes.
- Service: full swap moves the window correctly and carries payment fields; validation
  rejections (ineligible, already sponsored, same student, ended sponsorship);
  compensation path when the second command fails.
- Projection: after swap, old student is available again
  (`max_sponsorship_date` in the past) and new student is unavailable; payment columns
  populated.
- Web: replace flow end-to-end via the report entry point; reason required.
