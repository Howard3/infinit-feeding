# Sponsorship Replacement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let admins replace the student on an active sponsorship with a new student for the remaining time (truncate the departed student's record, transfer the remaining window), with a required reason and full audit pointers.

**Architecture:** Event-sourced Go app (gosignal CQRS). Sponsorships live in each student aggregate's `SponsorshipHistory`. Replacement is one service-level operation that emits two events — a new `TruncateSponsorship` event on the old student and the existing `UpdateSponsorship` event (extended with a back-pointer) on the new student — with a compensation path because there is no cross-aggregate transaction. Projections flow through the existing `upsertSponsorshipProjections` delete-and-reinsert.

**Tech Stack:** Go 1.25, chi router, a-h/templ (server-rendered HTML + HTMX), protobuf events via buf, SQLite/libsql, goose migrations, gosignal event sourcing.

**Spec:** `docs/superpowers/specs/2026-07-06-sponsorship-replacement-design.md`

## Global Constraints

- Commits: NO `Co-Authored-By: Claude` trailer, ever (user preference).
- Never hand-edit generated files: anything under `gen/` or any `*_templ.go`. Regenerate instead.
- Proto regen: `cd events && buf generate` (buf is installed at /opt/homebrew/bin/buf).
- Templ regen: `go tool templ generate` from repo root.
- Run tests with: `go test ./internal/student/ ./internal/webapi/ -count=1`.
- gosignal version semantics: an event's `Version` must EQUAL the aggregate's current version at apply time (`SafeApply` then sets `Version+1`). Commands therefore carry `Version: <current aggregate version>`.
- All dates in this domain are midnight-UTC `time.Time` / `eda.Date{Year,Month,Day}` protos.
- The aggregate `TruncateSponsorship` event is an *end-date adjustment* (only requires `new_end_date >= start_date`); the "must shorten" rule lives in the service flow so compensation can re-extend.

---

### Task 1: Proto schema — TruncateSponsorship event + extended fields

**Files:**
- Modify: `events/student.proto`
- Regenerate: `gen/go/eda/student.pb.go` (via buf; do not hand-edit)

**Interfaces:**
- Consumes: nothing (first task).
- Produces Go types used by all later tasks: `eda.Student_TruncateSponsorship`, `eda.Student_TruncateSponsorship_Event`, new fields `eda.Student_UpdateSponsorship.ReplacedStudentId`, `eda.Student_UpdateSponsorship_Event.ReplacedStudentId`, and `eda.Student_SponsorshipRecord.{ReplacedByStudentId, ReplacedStudentId, Reason}`.

- [ ] **Step 1: Edit `events/student.proto`**

Replace the existing `UpdateSponsorship` message (lines 204–220) with:

```proto
  message UpdateSponsorship {
    string sponsor_id = 1;
    Date start_date = 2;
    Date end_date = 3;
    uint64 version = 4;
    events.metadata.Metadata metadata = 5;
    string payment_id = 6;     // unique payment identifier
    double payment_amount = 7; // payment amount in USD
    string replaced_student_id = 8; // set when this sponsorship continues one truncated on another student

    message Event {
      string sponsor_id = 1;
      Date start_date = 2;
      Date end_date = 3;
      string payment_id = 4;
      double payment_amount = 5;
      string replaced_student_id = 6;
    }
  }

  // TruncateSponsorship adjusts the end date of an existing sponsorship record.
  // Used when an admin replaces the sponsored student (shorten), and by the
  // service compensation path (restore the original end date on failure).
  // The record is identified by payment_id when present, otherwise by
  // sponsor_id + start_date.
  message TruncateSponsorship {
    string sponsor_id = 1;
    Date start_date = 2;
    string payment_id = 3;
    Date new_end_date = 4;
    string replaced_by_student_id = 5; // audit pointer to the replacement student
    string reason = 6;                 // why the sponsorship was truncated
    uint64 version = 7;
    events.metadata.Metadata metadata = 8;

    message Event {
      string sponsor_id = 1;
      Date start_date = 2;
      string payment_id = 3;
      Date new_end_date = 4;
      string replaced_by_student_id = 5;
      string reason = 6;
    }
  }
```

Replace the existing `SponsorshipRecord` message (lines 222–228) with:

```proto
  message SponsorshipRecord {
    string sponsor_id = 1;
    Date start_date = 2;
    Date end_date = 3;
    string payment_id = 4;
    double payment_amount = 5;
    string replaced_by_student_id = 6; // set when this record was truncated in favor of another student
    string replaced_student_id = 7;    // set when this record continues one truncated on another student
    string reason = 8;                 // reason the truncation/replacement happened
  }
```

- [ ] **Step 2: Regenerate and build**

Run: `cd events && buf generate && cd .. && go build ./...`
Expected: no output, exit 0. Verify new types exist: `grep -c "TruncateSponsorship" gen/go/eda/student.pb.go` prints a number > 0.

- [ ] **Step 3: Commit**

```bash
git add events/student.proto gen/go/eda/student.pb.go
git commit -m "Add TruncateSponsorship event and replacement audit fields to student proto"
```

---

### Task 2: Aggregate — persist payment fields (existing bug fix)

**Files:**
- Modify: `internal/student/aggregate.go:714-724` (`UpdateSponsorship`) and `internal/student/aggregate.go:574-591` (`handleUpdateSponsorship`)
- Create: `internal/student/sponsorship_aggregate_test.go`

**Interfaces:**
- Consumes: Task 1 proto fields.
- Produces: `SponsorshipRecord`s in aggregate state now carry `PaymentId`, `PaymentAmount`, `ReplacedStudentId`. Test helper `newSponsorTestAggregate(t *testing.T) *Aggregate` used by Task 3 tests in the same file.

- [ ] **Step 1: Write the failing test**

Create `internal/student/sponsorship_aggregate_test.go`:

```go
package student

import (
	"testing"

	"geevly/gen/go/eda"
)

// newSponsorTestAggregate creates a student aggregate with one applied Create
// event. After creation the aggregate version is 1.
func newSponsorTestAggregate(t *testing.T) *Aggregate {
	t.Helper()
	agg := &Aggregate{}
	agg.SetIDUint64(1)
	if _, err := agg.CreateStudent(&eda.Student_Create{
		FirstName:   "Test",
		LastName:    "Student",
		DateOfBirth: &eda.Date{Year: 2015, Month: 1, Day: 2},
	}); err != nil {
		t.Fatalf("create student: %v", err)
	}
	return agg
}

func TestUpdateSponsorship_PersistsPaymentAndReplacementFields(t *testing.T) {
	agg := newSponsorTestAggregate(t)

	if _, err := agg.UpdateSponsorship(&eda.Student_UpdateSponsorship{
		SponsorId:         "sponsor-1",
		StartDate:         &eda.Date{Year: 2026, Month: 1, Day: 1},
		EndDate:           &eda.Date{Year: 2026, Month: 12, Day: 31},
		PaymentId:         "pay-123",
		PaymentAmount:     99.5,
		ReplacedStudentId: "42",
		Version:           agg.GetVersion(),
	}); err != nil {
		t.Fatalf("update sponsorship: %v", err)
	}

	recs := agg.GetStudent().GetSponsorshipHistory()
	if len(recs) != 1 {
		t.Fatalf("expected 1 sponsorship record, got %d", len(recs))
	}
	if recs[0].PaymentId != "pay-123" {
		t.Errorf("PaymentId not persisted: got %q", recs[0].PaymentId)
	}
	if recs[0].PaymentAmount != 99.5 {
		t.Errorf("PaymentAmount not persisted: got %v", recs[0].PaymentAmount)
	}
	if recs[0].ReplacedStudentId != "42" {
		t.Errorf("ReplacedStudentId not persisted: got %q", recs[0].ReplacedStudentId)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/student/ -run TestUpdateSponsorship_PersistsPaymentAndReplacementFields -v`
Expected: FAIL with `PaymentId not persisted: got ""` (the event and record currently drop these fields).

- [ ] **Step 3: Fix the command and handler**

In `internal/student/aggregate.go`, replace `UpdateSponsorship` (line ~714):

```go
func (sd *Aggregate) UpdateSponsorship(cmd *eda.Student_UpdateSponsorship) (*gosignal.Event, error) {
	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_UPDATE_SPONSORSHIP,
		data: &eda.Student_UpdateSponsorship_Event{
			SponsorId:         cmd.SponsorId,
			StartDate:         cmd.StartDate,
			EndDate:           cmd.EndDate,
			PaymentId:         cmd.PaymentId,
			PaymentAmount:     cmd.PaymentAmount,
			ReplacedStudentId: cmd.ReplacedStudentId,
		},
		version: cmd.GetVersion(),
	})
}
```

And in `handleUpdateSponsorship` (line ~574), replace the record construction:

```go
	// Create new sponsorship record
	newRecord := &eda.Student_SponsorshipRecord{
		SponsorId:         data.SponsorId,
		StartDate:         data.StartDate,
		EndDate:           data.EndDate,
		PaymentId:         data.PaymentId,
		PaymentAmount:     data.PaymentAmount,
		ReplacedStudentId: data.ReplacedStudentId,
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/student/ -run TestUpdateSponsorship_PersistsPaymentAndReplacementFields -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/student/aggregate.go internal/student/sponsorship_aggregate_test.go
git commit -m "Fix UpdateSponsorship dropping payment fields; carry replacement pointer"
```

---

### Task 3: Aggregate — TruncateSponsorship command, handler, replay

**Files:**
- Modify: `internal/student/aggregate.go` (new const, routeEvent case, errors, helpers, command, handler)
- Modify: `internal/student/event_handlers.go:147-148` (route new event to sponsorship projection handler)
- Modify: `internal/student/service.go:42-74` (`RunCommand` dispatch case)
- Test: `internal/student/sponsorship_aggregate_test.go` (extend)

**Interfaces:**
- Consumes: `newSponsorTestAggregate` from Task 2; Task 1 proto types.
- Produces (used by Tasks 5–6):
  - `const EVENT_TRUNCATE_SPONSORSHIP = "TruncateSponsorship"`
  - `var ErrSponsorshipNotFound`, `var ErrAmbiguousSponsorship` (package `student`)
  - `func (sd *Aggregate) FindSponsorshipRecord(paymentID, sponsorID string, startDate *eda.Date) (*eda.Student_SponsorshipRecord, error)`
  - `func (sd *Aggregate) TruncateSponsorship(cmd *eda.Student_TruncateSponsorship) (*gosignal.Event, error)`
  - `func protoDateToTime(d *eda.Date) time.Time` (package-private helper)
  - `RunCommand` accepts `*eda.Student_TruncateSponsorship`

- [ ] **Step 1: Write the failing tests**

Append to `internal/student/sponsorship_aggregate_test.go` (add `"github.com/Howard3/gosignal"`, `"errors"`, and `"google.golang.org/protobuf/proto"` to imports):

```go
func sponsorAggWithRecord(t *testing.T) *Aggregate {
	t.Helper()
	agg := newSponsorTestAggregate(t)
	if _, err := agg.UpdateSponsorship(&eda.Student_UpdateSponsorship{
		SponsorId:     "sponsor-1",
		StartDate:     &eda.Date{Year: 2026, Month: 1, Day: 1},
		EndDate:       &eda.Date{Year: 2026, Month: 12, Day: 31},
		PaymentId:     "pay-123",
		PaymentAmount: 50,
		Version:       agg.GetVersion(),
	}); err != nil {
		t.Fatalf("update sponsorship: %v", err)
	}
	return agg
}

func TestTruncateSponsorship_HappyPath(t *testing.T) {
	agg := sponsorAggWithRecord(t)

	if _, err := agg.TruncateSponsorship(&eda.Student_TruncateSponsorship{
		PaymentId:           "pay-123",
		NewEndDate:          &eda.Date{Year: 2026, Month: 7, Day: 7},
		ReplacedByStudentId: "99",
		Reason:              "Student moved away",
		Version:             agg.GetVersion(),
	}); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	rec := agg.GetStudent().GetSponsorshipHistory()[0]
	if rec.EndDate.Year != 2026 || rec.EndDate.Month != 7 || rec.EndDate.Day != 7 {
		t.Errorf("end date not truncated: got %v", rec.EndDate)
	}
	if rec.ReplacedByStudentId != "99" {
		t.Errorf("replaced_by pointer missing: got %q", rec.ReplacedByStudentId)
	}
	if rec.Reason != "Student moved away" {
		t.Errorf("reason missing: got %q", rec.Reason)
	}
}

func TestTruncateSponsorship_LegacyMatchBySponsorAndStartDate(t *testing.T) {
	agg := newSponsorTestAggregate(t)
	// legacy record: no payment id (pre-fix production data)
	if _, err := agg.UpdateSponsorship(&eda.Student_UpdateSponsorship{
		SponsorId: "sponsor-1",
		StartDate: &eda.Date{Year: 2026, Month: 2, Day: 1},
		EndDate:   &eda.Date{Year: 2026, Month: 12, Day: 31},
		Version:   agg.GetVersion(),
	}); err != nil {
		t.Fatalf("update sponsorship: %v", err)
	}

	if _, err := agg.TruncateSponsorship(&eda.Student_TruncateSponsorship{
		SponsorId:  "sponsor-1",
		StartDate:  &eda.Date{Year: 2026, Month: 2, Day: 1},
		NewEndDate: &eda.Date{Year: 2026, Month: 7, Day: 7},
		Reason:     "Graduated",
		Version:    agg.GetVersion(),
	}); err != nil {
		t.Fatalf("truncate by sponsor+start: %v", err)
	}

	rec := agg.GetStudent().GetSponsorshipHistory()[0]
	if rec.EndDate.Month != 7 {
		t.Errorf("legacy record not truncated: got %v", rec.EndDate)
	}
}

func TestTruncateSponsorship_RecordNotFound(t *testing.T) {
	agg := sponsorAggWithRecord(t)
	_, err := agg.TruncateSponsorship(&eda.Student_TruncateSponsorship{
		PaymentId:  "no-such-payment",
		NewEndDate: &eda.Date{Year: 2026, Month: 7, Day: 7},
		Reason:     "x",
		Version:    agg.GetVersion(),
	})
	if !errors.Is(err, ErrSponsorshipNotFound) {
		t.Fatalf("expected ErrSponsorshipNotFound, got %v", err)
	}
}

func TestTruncateSponsorship_AmbiguousMatchRefused(t *testing.T) {
	agg := newSponsorTestAggregate(t)
	for i := 0; i < 2; i++ {
		if _, err := agg.UpdateSponsorship(&eda.Student_UpdateSponsorship{
			SponsorId: "sponsor-1",
			StartDate: &eda.Date{Year: 2026, Month: 3, Day: 1},
			EndDate:   &eda.Date{Year: 2026, Month: 12, Day: 31},
			Version:   agg.GetVersion(),
		}); err != nil {
			t.Fatalf("update sponsorship %d: %v", i, err)
		}
	}

	_, err := agg.TruncateSponsorship(&eda.Student_TruncateSponsorship{
		SponsorId:  "sponsor-1",
		StartDate:  &eda.Date{Year: 2026, Month: 3, Day: 1},
		NewEndDate: &eda.Date{Year: 2026, Month: 7, Day: 7},
		Reason:     "x",
		Version:    agg.GetVersion(),
	})
	if !errors.Is(err, ErrAmbiguousSponsorship) {
		t.Fatalf("expected ErrAmbiguousSponsorship, got %v", err)
	}
}

func TestTruncateSponsorship_EndBeforeStartRefused(t *testing.T) {
	agg := sponsorAggWithRecord(t)
	_, err := agg.TruncateSponsorship(&eda.Student_TruncateSponsorship{
		PaymentId:  "pay-123",
		NewEndDate: &eda.Date{Year: 2025, Month: 12, Day: 31}, // before 2026-01-01 start
		Reason:     "x",
		Version:    agg.GetVersion(),
	})
	if err == nil {
		t.Fatal("expected error for new end date before start date")
	}
}

// Replay: rehydrating from the raw event stream must reproduce truncated state.
func TestTruncateSponsorship_Replay(t *testing.T) {
	src := &Aggregate{}
	src.SetIDUint64(7)
	var events []*gosignal.Event

	e, err := src.CreateStudent(&eda.Student_Create{
		FirstName: "R", LastName: "P",
		DateOfBirth: &eda.Date{Year: 2015, Month: 1, Day: 2},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	events = append(events, e)

	e, err = src.UpdateSponsorship(&eda.Student_UpdateSponsorship{
		SponsorId: "sponsor-1",
		StartDate: &eda.Date{Year: 2026, Month: 1, Day: 1},
		EndDate:   &eda.Date{Year: 2026, Month: 12, Day: 31},
		PaymentId: "pay-9",
		Version:   src.GetVersion(),
	})
	if err != nil {
		t.Fatalf("sponsor: %v", err)
	}
	events = append(events, e)

	e, err = src.TruncateSponsorship(&eda.Student_TruncateSponsorship{
		PaymentId:           "pay-9",
		NewEndDate:          &eda.Date{Year: 2026, Month: 7, Day: 7},
		ReplacedByStudentId: "8",
		Reason:              "Student left school",
		Version:             src.GetVersion(),
	})
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	events = append(events, e)

	replayed := &Aggregate{}
	replayed.SetIDUint64(7)
	for i, evt := range events {
		if err := replayed.Apply(*evt); err != nil {
			t.Fatalf("replay event %d (%s): %v", i, evt.Type, err)
		}
	}

	rec := replayed.GetStudent().GetSponsorshipHistory()[0]
	if rec.EndDate.Month != 7 || rec.EndDate.Day != 7 {
		t.Errorf("replayed end date wrong: %v", rec.EndDate)
	}
	if rec.ReplacedByStudentId != "8" || rec.Reason != "Student left school" {
		t.Errorf("replayed audit fields wrong: %q %q", rec.ReplacedByStudentId, rec.Reason)
	}
}

// Old serialized events (bytes written before the new proto fields existed)
// must still deserialize and apply. Proto3 makes absent fields zero-valued;
// simulate an old event by marshalling only the fields that existed then.
func TestUpdateSponsorship_OldEventBytesStillApply(t *testing.T) {
	oldEvt := &eda.Student_UpdateSponsorship_Event{
		SponsorId: "sponsor-1",
		StartDate: &eda.Date{Year: 2025, Month: 1, Day: 1},
		EndDate:   &eda.Date{Year: 2025, Month: 12, Day: 31},
	}
	data, err := proto.Marshal(oldEvt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	agg := newSponsorTestAggregate(t)
	if err := agg.Apply(gosignal.Event{
		Type:        EVENT_UPDATE_SPONSORSHIP,
		Data:        data,
		Version:     agg.GetVersion(),
		AggregateID: agg.GetID(),
	}); err != nil {
		t.Fatalf("apply old-format event: %v", err)
	}

	rec := agg.GetStudent().GetSponsorshipHistory()[0]
	if rec.PaymentId != "" || rec.ReplacedStudentId != "" {
		t.Errorf("expected zero values for absent old fields, got %q %q", rec.PaymentId, rec.ReplacedStudentId)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/student/ -run TestTruncateSponsorship -v`
Expected: compile error — `agg.TruncateSponsorship undefined`, `ErrSponsorshipNotFound` undefined.

- [ ] **Step 3: Implement in `internal/student/aggregate.go`**

Add after the other error vars (line ~23):

```go
var ErrSponsorshipNotFound = fmt.Errorf("sponsorship record not found")
var ErrAmbiguousSponsorship = fmt.Errorf("multiple sponsorship records match; manual repair required")
```

Add after the other event consts (line ~39):

```go
const EVENT_TRUNCATE_SPONSORSHIP = "TruncateSponsorship"
```

Add a case to `routeEvent`'s switch (after the `EVENT_UPDATE_SPONSORSHIP` case, line ~163):

```go
	case EVENT_TRUNCATE_SPONSORSHIP:
		eventData = &eda.Student_TruncateSponsorship_Event{}
		handler = sd.handleTruncateSponsorship
```

Add near `MaxSponsorshipDate` (line ~662):

```go
// protoDateToTime converts an eda.Date to a midnight-UTC time.Time.
func protoDateToTime(d *eda.Date) time.Time {
	return time.Date(int(d.GetYear()), time.Month(d.GetMonth()), int(d.GetDay()), 0, 0, 0, 0, time.UTC)
}

// FindSponsorshipRecord locates a sponsorship record by payment ID when
// present, otherwise by sponsor ID + start date (all pre-payment-fix records
// have no payment ID, so the fallback is the common path for existing data).
// Refuses ambiguous matches rather than guessing.
func (sd *Aggregate) FindSponsorshipRecord(paymentID, sponsorID string, startDate *eda.Date) (*eda.Student_SponsorshipRecord, error) {
	var matches []*eda.Student_SponsorshipRecord
	for _, rec := range sd.data.GetSponsorshipHistory() {
		if paymentID != "" {
			if rec.PaymentId == paymentID {
				matches = append(matches, rec)
			}
			continue
		}
		if rec.SponsorId == sponsorID &&
			rec.GetStartDate().GetYear() == startDate.GetYear() &&
			rec.GetStartDate().GetMonth() == startDate.GetMonth() &&
			rec.GetStartDate().GetDay() == startDate.GetDay() {
			matches = append(matches, rec)
		}
	}
	switch len(matches) {
	case 0:
		return nil, ErrSponsorshipNotFound
	case 1:
		return matches[0], nil
	default:
		return nil, ErrAmbiguousSponsorship
	}
}
```

Add after `UpdateSponsorship` (line ~724):

```go
// TruncateSponsorship adjusts the end date of an existing sponsorship record.
// The event only requires new_end_date >= the record's start date; the
// admin-flow rule that a truncation must shorten the record is enforced by
// the service layer (compensation reuses this event to restore a later date).
func (sd *Aggregate) TruncateSponsorship(cmd *eda.Student_TruncateSponsorship) (*gosignal.Event, error) {
	if sd.data == nil {
		return nil, ErrStudentNotFound
	}

	rec, err := sd.FindSponsorshipRecord(cmd.PaymentId, cmd.SponsorId, cmd.StartDate)
	if err != nil {
		return nil, err
	}

	if protoDateToTime(cmd.NewEndDate).Before(protoDateToTime(rec.StartDate)) {
		return nil, fmt.Errorf("new end date %v precedes sponsorship start date %v", cmd.NewEndDate, rec.StartDate)
	}

	return sd.ApplyEvent(StudentEvent{
		eventType: EVENT_TRUNCATE_SPONSORSHIP,
		data: &eda.Student_TruncateSponsorship_Event{
			SponsorId:           cmd.SponsorId,
			StartDate:           cmd.StartDate,
			PaymentId:           cmd.PaymentId,
			NewEndDate:          cmd.NewEndDate,
			ReplacedByStudentId: cmd.ReplacedByStudentId,
			Reason:              cmd.Reason,
		},
		version: cmd.GetVersion(),
	})
}
```

Add after `handleUpdateSponsorship` (line ~591):

```go
func (sd *Aggregate) handleTruncateSponsorship(evt wrappedEvent) error {
	data := evt.data.(*eda.Student_TruncateSponsorship_Event)

	rec, err := sd.FindSponsorshipRecord(data.PaymentId, data.SponsorId, data.StartDate)
	if err != nil {
		return err
	}

	rec.EndDate = data.NewEndDate
	rec.ReplacedByStudentId = data.ReplacedByStudentId
	rec.Reason = data.Reason

	return nil
}
```

- [ ] **Step 4: Route the event to projections and RunCommand**

In `internal/student/event_handlers.go` line 147, extend the case:

```go
	case EVENT_UPDATE_SPONSORSHIP, EVENT_TRUNCATE_SPONSORSHIP:
		eh.handleUpdateSponsorshipEvent(ctx, id)
```

In `internal/student/service.go` `RunCommand` switch (after the `*eda.Student_UpdateSponsorship` case, line ~69):

```go
		case *eda.Student_TruncateSponsorship:
			return agg.TruncateSponsorship(cmd)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/student/ -run 'TestTruncateSponsorship|TestUpdateSponsorship' -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/student/aggregate.go internal/student/event_handlers.go internal/student/service.go internal/student/sponsorship_aggregate_test.go
git commit -m "Add TruncateSponsorship command, handler, and event routing"
```

---

### Task 4: Projection — persist payment columns; integration test setup

**Files:**
- Modify: `internal/student/repository.go:1494-1502` (INSERT in `upsertSponsorshipProjections`)
- Create: `internal/student/sponsorship_integration_test.go`

**Interfaces:**
- Consumes: aggregate/record fields from Tasks 2–3.
- Produces: `student_sponsorship_projections` rows now populate `payment_id`/`payment_amount`. Test helpers used by Task 5 (same file): `newTestStudentService(t) (*StudentService, *sqlRepository)` and `createSponsorableStudent(t, svc, firstName) uint64`.

- [ ] **Step 1: Write the failing test**

Create `internal/student/sponsorship_integration_test.go`. Note `NewRepository` panics on failure (no error return), uses the same libsql driver as production against a temp file, and `GetAllCurrentSponsorships` already scans payment columns NULL-safely — we use it to assert.

```go
package student

import (
	"context"
	"testing"
	"time"

	"geevly/gen/go/eda"
	"geevly/internal/infrastructure"

	"github.com/Howard3/gosignal/drivers/queue"
)

// newTestStudentService spins up the real repository (migrations included)
// against a temp-dir SQLite file and returns the service plus the concrete
// repo for synchronous projection updates in tests.
func newTestStudentService(t *testing.T) (*StudentService, *sqlRepository) {
	t.Helper()
	conn := infrastructure.SQLConnection{
		Type: "libsql",
		URI:  "file:" + t.TempDir() + "/test.db",
	}
	repo := NewRepository(conn, &queue.MemoryQueue{})
	svc := NewStudentService(repo, nil) // ACL is only used by Enroll/SetProfilePhoto commands
	return svc, repo.(*sqlRepository)
}

// createSponsorableStudent creates an ACTIVE, sponsorship-eligible student
// and returns its aggregate ID.
func createSponsorableStudent(t *testing.T, svc *StudentService, firstName string) uint64 {
	t.Helper()
	ctx := context.Background()

	agg, err := svc.CreateStudent(ctx, &eda.Student_Create{
		FirstName:   firstName,
		LastName:    "Test",
		DateOfBirth: &eda.Date{Year: 2015, Month: 1, Day: 2},
	})
	if err != nil {
		t.Fatalf("create student: %v", err)
	}
	id := agg.GetIDUint64()

	agg, err = svc.RunCommand(ctx, id, &eda.Student_SetStatus{
		Status:  eda.Student_ACTIVE,
		Version: agg.Version,
	})
	if err != nil {
		t.Fatalf("set status: %v", err)
	}

	if _, err := svc.RunCommand(ctx, id, &eda.Student_SetEligibility{
		Eligible: true,
		Version:  agg.Version,
	}); err != nil {
		t.Fatalf("set eligibility: %v", err)
	}

	return id
}

// protoDate converts a time.Time to an eda.Date.
func protoDate(t time.Time) *eda.Date {
	return &eda.Date{Year: int32(t.Year()), Month: int32(t.Month()), Day: int32(t.Day())}
}

func TestSponsorshipProjection_PersistsPaymentColumns(t *testing.T) {
	svc, repo := newTestStudentService(t)
	ctx := context.Background()

	id := createSponsorableStudent(t, svc, "Payment")
	agg, err := svc.GetStudent(ctx, id)
	if err != nil {
		t.Fatalf("get student: %v", err)
	}

	start := time.Now().UTC().AddDate(0, 0, -1)
	end := time.Now().UTC().AddDate(0, 6, 0)
	agg, err = svc.RunCommand(ctx, id, &eda.Student_UpdateSponsorship{
		SponsorId:     "sponsor-pay",
		StartDate:     protoDate(start),
		EndDate:       protoDate(end),
		PaymentId:     "pay-777",
		PaymentAmount: 120.25,
		Version:       agg.Version,
	})
	if err != nil {
		t.Fatalf("sponsor: %v", err)
	}

	// project synchronously (the event-handler path is async)
	if err := repo.upsertSponsorshipProjections(agg); err != nil {
		t.Fatalf("upsert projections: %v", err)
	}

	sponsorships, err := repo.GetAllCurrentSponsorships(ctx)
	if err != nil {
		t.Fatalf("get current sponsorships: %v", err)
	}
	if len(sponsorships) != 1 {
		t.Fatalf("expected 1 current sponsorship, got %d", len(sponsorships))
	}
	if sponsorships[0].PaymentID != "pay-777" {
		t.Errorf("payment_id column not populated: got %q", sponsorships[0].PaymentID)
	}
	if sponsorships[0].PaymentAmount != 120.25 {
		t.Errorf("payment_amount column not populated: got %v", sponsorships[0].PaymentAmount)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/student/ -run TestSponsorshipProjection_PersistsPaymentColumns -v`
Expected: FAIL with `payment_id column not populated: got ""`.

- [ ] **Step 3: Fix the INSERT**

In `internal/student/repository.go` `upsertSponsorshipProjections` (line ~1494), replace the INSERT statement:

```go
		_, err = tx.Exec(`
			INSERT INTO student_sponsorship_projections
			(student_id, sponsor_id, start_date, end_date, payment_id, payment_amount)
			VALUES (?, ?, ?, ?, ?, ?)
		`, student.GetID(), sponsorship.SponsorId, startDate, endDate,
			sponsorship.PaymentId, sponsorship.PaymentAmount)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/student/ -run TestSponsorshipProjection_PersistsPaymentColumns -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/student/repository.go internal/student/sponsorship_integration_test.go
git commit -m "Persist payment columns in sponsorship projections; add integration test harness"
```

---

### Task 5: Service — ReplaceSponsorship with validation and compensation

**Files:**
- Create: `internal/student/sponsorship_replace.go`
- Modify: `internal/student/service.go:16-20` (add test-hook field to struct)
- Test: `internal/student/sponsorship_integration_test.go` (extend)

**Interfaces:**
- Consumes: Task 3 aggregate API, Task 4 test helpers.
- Produces (used by Task 6 web layer):

```go
type ReplaceSponsorshipParams struct {
	OldStudentID    uint64
	NewStudentID    uint64
	SponsorID       string
	StartDate       time.Time // identifies the record (with SponsorID) when PaymentID is empty
	PaymentID       string    // primary record identifier when present
	Reason          string    // required
	ReplacementDate time.Time // normally time.Now(); parameterized for testability
}
func (s *StudentService) ReplaceSponsorship(ctx context.Context, p ReplaceSponsorshipParams) error
// errors: ErrReasonRequired, ErrSponsorshipEnded, ErrReplacementStudentInvalid,
//         plus ErrSponsorshipNotFound / ErrAmbiguousSponsorship from the aggregate.
```

- [ ] **Step 1: Add the test hook field**

In `internal/student/service.go`, extend the struct (line ~16):

```go
type StudentService struct {
	repo          Repository
	eventHandlers *eventHandlers
	acl           AntiCorruptionLayer

	// beforeTransferHook is a test seam: called after the truncate command
	// succeeds and before the transfer command runs. Nil in production.
	beforeTransferHook func() error
}
```

- [ ] **Step 2: Write the failing tests**

Append to `internal/student/sponsorship_integration_test.go` (add `"errors"` and `"strings"` to imports if the compiler asks; `sponsorStudent` helper included here):

```go
// sponsorStudent applies an UpdateSponsorship command and synchronously
// projects it. Returns the updated aggregate.
func sponsorStudent(t *testing.T, svc *StudentService, repo *sqlRepository, id uint64, sponsorID, paymentID string, start, end time.Time) *Aggregate {
	t.Helper()
	ctx := context.Background()
	agg, err := svc.GetStudent(ctx, id)
	if err != nil {
		t.Fatalf("get student: %v", err)
	}
	agg, err = svc.RunCommand(ctx, id, &eda.Student_UpdateSponsorship{
		SponsorId:     sponsorID,
		StartDate:     protoDate(start),
		EndDate:       protoDate(end),
		PaymentId:     paymentID,
		PaymentAmount: 100,
		Version:       agg.Version,
	})
	if err != nil {
		t.Fatalf("sponsor student %d: %v", id, err)
	}
	if err := repo.upsertSponsorshipProjections(agg); err != nil {
		t.Fatalf("project sponsorships: %v", err)
	}
	return agg
}

func TestReplaceSponsorship_HappyPath(t *testing.T) {
	svc, repo := newTestStudentService(t)
	ctx := context.Background()

	oldID := createSponsorableStudent(t, svc, "Old")
	newID := createSponsorableStudent(t, svc, "New")

	start := time.Now().UTC().AddDate(0, -1, 0)
	end := time.Now().UTC().AddDate(0, 5, 0)
	sponsorStudent(t, svc, repo, oldID, "sponsor-1", "pay-1", start, end)

	today := time.Now().UTC()
	err := svc.ReplaceSponsorship(ctx, ReplaceSponsorshipParams{
		OldStudentID:    oldID,
		NewStudentID:    newID,
		SponsorID:       "sponsor-1",
		StartDate:       start,
		PaymentID:       "pay-1",
		Reason:          "Student left school",
		ReplacementDate: today,
	})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}

	// Old student: record truncated to today, audit fields set.
	oldAgg, _ := svc.GetStudent(ctx, oldID)
	oldRec := oldAgg.GetStudent().GetSponsorshipHistory()[0]
	if protoDateToTime(oldRec.EndDate).Format("2006-01-02") != today.Format("2006-01-02") {
		t.Errorf("old record not truncated to today: %v", oldRec.EndDate)
	}
	if oldRec.Reason != "Student left school" {
		t.Errorf("reason not stored: %q", oldRec.Reason)
	}
	newAgg, _ := svc.GetStudent(ctx, newID)
	if oldRec.ReplacedByStudentId != newAgg.GetID() {
		t.Errorf("replaced_by pointer wrong: %q", oldRec.ReplacedByStudentId)
	}

	// New student: remaining window, payment carried, back-pointer set.
	newRec := newAgg.GetStudent().GetSponsorshipHistory()[0]
	if protoDateToTime(newRec.StartDate).Format("2006-01-02") != today.Format("2006-01-02") {
		t.Errorf("new record start wrong: %v", newRec.StartDate)
	}
	if protoDateToTime(newRec.EndDate).Format("2006-01-02") != end.Format("2006-01-02") {
		t.Errorf("new record end wrong: %v", newRec.EndDate)
	}
	if newRec.PaymentId != "pay-1" || newRec.PaymentAmount != 100 {
		t.Errorf("payment not carried over: %q %v", newRec.PaymentId, newRec.PaymentAmount)
	}
	if newRec.ReplacedStudentId != oldAgg.GetID() {
		t.Errorf("back-pointer wrong: %q", newRec.ReplacedStudentId)
	}

	// Sponsor-facing view: exactly one current sponsorship, pointing at the new student.
	current, err := svc.GetCurrentSponsorships(ctx, "sponsor-1")
	if err != nil {
		t.Fatalf("current sponsorships: %v", err)
	}
	var currentIDs []string
	for _, sp := range current {
		currentIDs = append(currentIDs, sp.StudentID)
	}
	if len(current) != 1 || current[0].StudentID != newAgg.GetID() {
		t.Errorf("expected exactly [%s] current, got %v", newAgg.GetID(), currentIDs)
	}

	// Availability: old student free again, new student occupied.
	list, err := svc.ListStudents(ctx, 100, 1, ActiveOnly(), EligibleForSponsorshipOnly())
	if err != nil {
		t.Fatalf("list students: %v", err)
	}
	for _, st := range list.Students {
		if uint64(st.ID) == newID {
			t.Errorf("new student still listed as available for sponsorship")
		}
	}
}

func TestReplaceSponsorship_ValidationRejections(t *testing.T) {
	svc, repo := newTestStudentService(t)
	ctx := context.Background()

	oldID := createSponsorableStudent(t, svc, "Old")
	start := time.Now().UTC().AddDate(0, -1, 0)
	end := time.Now().UTC().AddDate(0, 5, 0)
	sponsorStudent(t, svc, repo, oldID, "sponsor-1", "pay-1", start, end)

	base := ReplaceSponsorshipParams{
		OldStudentID:    oldID,
		SponsorID:       "sponsor-1",
		StartDate:       start,
		PaymentID:       "pay-1",
		Reason:          "reason",
		ReplacementDate: time.Now().UTC(),
	}

	t.Run("missing reason", func(t *testing.T) {
		p := base
		p.NewStudentID = createSponsorableStudent(t, svc, "R1")
		p.Reason = "  "
		if err := svc.ReplaceSponsorship(ctx, p); !errors.Is(err, ErrReasonRequired) {
			t.Fatalf("expected ErrReasonRequired, got %v", err)
		}
	})

	t.Run("same student", func(t *testing.T) {
		p := base
		p.NewStudentID = oldID
		if err := svc.ReplaceSponsorship(ctx, p); err == nil {
			t.Fatal("expected error replacing student with itself")
		}
	})

	t.Run("ineligible replacement", func(t *testing.T) {
		p := base
		// active but never flagged eligible
		agg, err := svc.CreateStudent(ctx, &eda.Student_Create{
			FirstName: "NotEligible", LastName: "T",
			DateOfBirth: &eda.Date{Year: 2015, Month: 1, Day: 2},
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := svc.RunCommand(ctx, agg.GetIDUint64(), &eda.Student_SetStatus{
			Status: eda.Student_ACTIVE, Version: agg.Version,
		}); err != nil {
			t.Fatalf("activate: %v", err)
		}
		p.NewStudentID = agg.GetIDUint64()
		if err := svc.ReplaceSponsorship(ctx, p); !errors.Is(err, ErrReplacementStudentInvalid) {
			t.Fatalf("expected ErrReplacementStudentInvalid, got %v", err)
		}
	})

	t.Run("already sponsored replacement", func(t *testing.T) {
		p := base
		takenID := createSponsorableStudent(t, svc, "Taken")
		sponsorStudent(t, svc, repo, takenID, "sponsor-2", "pay-2", start, end)
		p.NewStudentID = takenID
		if err := svc.ReplaceSponsorship(ctx, p); !errors.Is(err, ErrReplacementStudentInvalid) {
			t.Fatalf("expected ErrReplacementStudentInvalid, got %v", err)
		}
	})

	t.Run("ended sponsorship", func(t *testing.T) {
		endedID := createSponsorableStudent(t, svc, "Ended")
		pastStart := time.Now().UTC().AddDate(-1, 0, 0)
		pastEnd := time.Now().UTC().AddDate(0, -6, 0)
		sponsorStudent(t, svc, repo, endedID, "sponsor-3", "pay-3", pastStart, pastEnd)
		p := ReplaceSponsorshipParams{
			OldStudentID:    endedID,
			NewStudentID:    createSponsorableStudent(t, svc, "R2"),
			SponsorID:       "sponsor-3",
			StartDate:       pastStart,
			PaymentID:       "pay-3",
			Reason:          "reason",
			ReplacementDate: time.Now().UTC(),
		}
		if err := svc.ReplaceSponsorship(ctx, p); !errors.Is(err, ErrSponsorshipEnded) {
			t.Fatalf("expected ErrSponsorshipEnded, got %v", err)
		}
	})

	t.Run("ends today is refused", func(t *testing.T) {
		todayID := createSponsorableStudent(t, svc, "EndsToday")
		sStart := time.Now().UTC().AddDate(0, -1, 0)
		sEnd := time.Now().UTC()
		sponsorStudent(t, svc, repo, todayID, "sponsor-4", "pay-4", sStart, sEnd)
		p := ReplaceSponsorshipParams{
			OldStudentID:    todayID,
			NewStudentID:    createSponsorableStudent(t, svc, "R3"),
			SponsorID:       "sponsor-4",
			StartDate:       sStart,
			PaymentID:       "pay-4",
			Reason:          "reason",
			ReplacementDate: time.Now().UTC(),
		}
		if err := svc.ReplaceSponsorship(ctx, p); !errors.Is(err, ErrSponsorshipEnded) {
			t.Fatalf("expected ErrSponsorshipEnded for same-day end, got %v", err)
		}
	})
}

func TestReplaceSponsorship_DoubleSubmitRefused(t *testing.T) {
	svc, repo := newTestStudentService(t)
	ctx := context.Background()

	oldID := createSponsorableStudent(t, svc, "Old")
	newID := createSponsorableStudent(t, svc, "New")
	start := time.Now().UTC().AddDate(0, -1, 0)
	end := time.Now().UTC().AddDate(0, 5, 0)
	sponsorStudent(t, svc, repo, oldID, "sponsor-1", "pay-1", start, end)

	p := ReplaceSponsorshipParams{
		OldStudentID: oldID, NewStudentID: newID,
		SponsorID: "sponsor-1", StartDate: start, PaymentID: "pay-1",
		Reason: "left", ReplacementDate: time.Now().UTC(),
	}
	if err := svc.ReplaceSponsorship(ctx, p); err != nil {
		t.Fatalf("first replace: %v", err)
	}

	// Second submit: the old record now ends today → refused; and even with a
	// different replacement, the first new student is no longer available.
	p.NewStudentID = createSponsorableStudent(t, svc, "Third")
	if err := svc.ReplaceSponsorship(ctx, p); !errors.Is(err, ErrSponsorshipEnded) {
		t.Fatalf("expected ErrSponsorshipEnded on double submit, got %v", err)
	}
}

func TestReplaceSponsorship_SameReplacementStudentTwiceRefused(t *testing.T) {
	svc, repo := newTestStudentService(t)
	ctx := context.Background()

	oldA := createSponsorableStudent(t, svc, "OldA")
	oldB := createSponsorableStudent(t, svc, "OldB")
	target := createSponsorableStudent(t, svc, "Target")
	start := time.Now().UTC().AddDate(0, -1, 0)
	end := time.Now().UTC().AddDate(0, 5, 0)
	sponsorStudent(t, svc, repo, oldA, "sponsor-1", "pay-A", start, end)
	sponsorStudent(t, svc, repo, oldB, "sponsor-2", "pay-B", start, end)

	mk := func(oldID uint64, sponsorID, payID string) ReplaceSponsorshipParams {
		return ReplaceSponsorshipParams{
			OldStudentID: oldID, NewStudentID: target,
			SponsorID: sponsorID, StartDate: start, PaymentID: payID,
			Reason: "left", ReplacementDate: time.Now().UTC(),
		}
	}

	if err := svc.ReplaceSponsorship(ctx, mk(oldA, "sponsor-1", "pay-A")); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	if err := svc.ReplaceSponsorship(ctx, mk(oldB, "sponsor-2", "pay-B")); !errors.Is(err, ErrReplacementStudentInvalid) {
		t.Fatalf("expected ErrReplacementStudentInvalid double-booking, got %v", err)
	}
}

// Degenerate case: a sponsorship that started today and is replaced today
// leaves a zero-length truncated record (start == end == today). This must
// succeed and must not break the sponsor's current-sponsorships query.
func TestReplaceSponsorship_SameDayStartAndReplace(t *testing.T) {
	svc, repo := newTestStudentService(t)
	ctx := context.Background()

	oldID := createSponsorableStudent(t, svc, "Old")
	newID := createSponsorableStudent(t, svc, "New")
	today := time.Now().UTC()
	end := today.AddDate(0, 6, 0)
	sponsorStudent(t, svc, repo, oldID, "sponsor-1", "pay-1", today, end)

	if err := svc.ReplaceSponsorship(ctx, ReplaceSponsorshipParams{
		OldStudentID: oldID, NewStudentID: newID,
		SponsorID: "sponsor-1", StartDate: today, PaymentID: "pay-1",
		Reason: "left", ReplacementDate: today,
	}); err != nil {
		t.Fatalf("same-day replace: %v", err)
	}

	oldAgg, _ := svc.GetStudent(ctx, oldID)
	rec := oldAgg.GetStudent().GetSponsorshipHistory()[0]
	if protoDateToTime(rec.StartDate) != protoDateToTime(rec.EndDate) {
		t.Errorf("expected zero-length truncated record, got %v → %v", rec.StartDate, rec.EndDate)
	}

	newAgg, _ := svc.GetStudent(ctx, newID)
	current, err := svc.GetCurrentSponsorships(ctx, "sponsor-1")
	if err != nil {
		t.Fatalf("current sponsorships must not error: %v", err)
	}
	found := false
	for _, sp := range current {
		if sp.StudentID == newAgg.GetID() {
			found = true
		}
	}
	if !found {
		t.Errorf("new student missing from sponsor's current sponsorships")
	}
}

func TestReplaceSponsorship_CompensationRestoresOldRecord(t *testing.T) {
	svc, repo := newTestStudentService(t)
	ctx := context.Background()

	oldID := createSponsorableStudent(t, svc, "Old")
	newID := createSponsorableStudent(t, svc, "New")
	start := time.Now().UTC().AddDate(0, -1, 0)
	end := time.Now().UTC().AddDate(0, 5, 0)
	sponsorStudent(t, svc, repo, oldID, "sponsor-1", "pay-1", start, end)

	svc.beforeTransferHook = func() error {
		return errors.New("simulated transfer failure")
	}
	defer func() { svc.beforeTransferHook = nil }()

	err := svc.ReplaceSponsorship(ctx, ReplaceSponsorshipParams{
		OldStudentID: oldID, NewStudentID: newID,
		SponsorID: "sponsor-1", StartDate: start, PaymentID: "pay-1",
		Reason: "left", ReplacementDate: time.Now().UTC(),
	})
	if err == nil || !strings.Contains(err.Error(), "simulated transfer failure") {
		t.Fatalf("expected transfer failure to surface, got %v", err)
	}

	// Old record restored to the original end date.
	oldAgg, _ := svc.GetStudent(ctx, oldID)
	rec := oldAgg.GetStudent().GetSponsorshipHistory()[0]
	if protoDateToTime(rec.EndDate).Format("2006-01-02") != end.Format("2006-01-02") {
		t.Errorf("old record not restored: end = %v", rec.EndDate)
	}
	if rec.ReplacedByStudentId != "" {
		t.Errorf("replaced_by should be cleared after compensation: %q", rec.ReplacedByStudentId)
	}

	// New student untouched.
	newAgg, _ := svc.GetStudent(ctx, newID)
	if len(newAgg.GetStudent().GetSponsorshipHistory()) != 0 {
		t.Errorf("new student should have no sponsorship after failed transfer")
	}
}

func TestReplaceSponsorship_ImpactAttribution(t *testing.T) {
	svc, repo := newTestStudentService(t)
	ctx := context.Background()

	oldID := createSponsorableStudent(t, svc, "Old")
	newID := createSponsorableStudent(t, svc, "New")

	start := time.Now().UTC().AddDate(0, 0, -30)
	end := time.Now().UTC().AddDate(0, 5, 0)
	replacement := time.Now().UTC().AddDate(0, 0, -10)
	sponsorStudent(t, svc, repo, oldID, "sponsor-1", "pay-1", start, end)

	feed := func(id uint64, when time.Time) {
		t.Helper()
		agg, err := svc.GetStudent(ctx, id)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		agg, err = svc.RunCommand(ctx, id, &eda.Student_Feeding{
			UnixTimestamp: uint64(when.Unix()),
			Version:       agg.Version,
		})
		if err != nil {
			t.Fatalf("feed %d: %v", id, err)
		}
		if err := repo.upsertFeedingEventProjection(agg); err != nil {
			t.Fatalf("project feeding: %v", err)
		}
	}

	// Old student fed before AND after the replacement date (both in the past).
	feed(oldID, time.Now().UTC().AddDate(0, 0, -20)) // inside old window
	feed(oldID, time.Now().UTC().AddDate(0, 0, -5))  // after replacement → not attributable
	// New student fed after the replacement date.
	feed(newID, time.Now().UTC().AddDate(0, 0, -3))

	if err := svc.ReplaceSponsorship(ctx, ReplaceSponsorshipParams{
		OldStudentID: oldID, NewStudentID: newID,
		SponsorID: "sponsor-1", StartDate: start, PaymentID: "pay-1",
		Reason: "left", ReplacementDate: replacement,
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	meals, err := svc.GetSponsorImpactMetrics(ctx, "sponsor-1")
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	// old-before-replacement (1) + new-after-replacement (1); the old
	// student's post-replacement feeding must NOT count.
	if meals != 2 {
		t.Errorf("expected 2 attributed meals, got %d", meals)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/student/ -run TestReplaceSponsorship -v`
Expected: compile error — `ReplaceSponsorshipParams` undefined.

- [ ] **Step 4: Implement `internal/student/sponsorship_replace.go`**

```go
package student

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"geevly/gen/go/eda"
)

var ErrReasonRequired = fmt.Errorf("a reason is required to replace a sponsorship")
var ErrSponsorshipEnded = fmt.Errorf("sponsorship has already ended")
var ErrReplacementStudentInvalid = fmt.Errorf("replacement student must be active, eligible, and not currently sponsored")

// ReplaceSponsorshipParams identifies a sponsorship on the old student and
// the student who takes over the remaining window.
type ReplaceSponsorshipParams struct {
	OldStudentID    uint64
	NewStudentID    uint64
	SponsorID       string
	StartDate       time.Time // identifies the record (with SponsorID) when PaymentID is empty
	PaymentID       string    // primary record identifier when present
	Reason          string    // required; stored on the truncated record
	ReplacementDate time.Time // normally time.Now(); the truncation/transfer boundary
}

// ReplaceSponsorship truncates the identified sponsorship on the old student
// at the replacement date and transfers the remaining window to the new
// student. Two events, one per aggregate; there is no cross-aggregate
// transaction, so a failed transfer is compensated by restoring the old
// record's original end date.
func (s *StudentService) ReplaceSponsorship(ctx context.Context, p ReplaceSponsorshipParams) error {
	if strings.TrimSpace(p.Reason) == "" {
		return ErrReasonRequired
	}
	if p.OldStudentID == p.NewStudentID {
		return fmt.Errorf("replacement student must differ from the currently sponsored student")
	}

	oldAgg, err := s.repo.loadStudent(ctx, p.OldStudentID)
	if err != nil {
		return fmt.Errorf("failed to load sponsored student: %w", err)
	}

	startDate := &eda.Date{
		Year:  int32(p.StartDate.Year()),
		Month: int32(p.StartDate.Month()),
		Day:   int32(p.StartDate.Day()),
	}
	record, err := oldAgg.FindSponsorshipRecord(p.PaymentID, p.SponsorID, startDate)
	if err != nil {
		return err
	}

	replacementDate := time.Date(
		p.ReplacementDate.Year(), p.ReplacementDate.Month(), p.ReplacementDate.Day(),
		0, 0, 0, 0, time.UTC,
	)
	if !protoDateToTime(record.EndDate).After(replacementDate) {
		return ErrSponsorshipEnded
	}

	newAgg, err := s.repo.loadStudent(ctx, p.NewStudentID)
	if err != nil {
		return fmt.Errorf("failed to load replacement student: %w", err)
	}
	maxDate := newAgg.MaxSponsorshipDate()
	available := maxDate == nil || maxDate.Before(time.Now())
	if !newAgg.IsActive() || !newAgg.GetStudent().GetEligibleForSponsorship() || !available {
		return ErrReplacementStudentInvalid
	}

	originalEnd := record.EndDate
	paymentAmount := record.PaymentAmount
	newEndDate := &eda.Date{
		Year:  int32(replacementDate.Year()),
		Month: int32(replacementDate.Month()),
		Day:   int32(replacementDate.Day()),
	}

	// 1) truncate the old student's record at the replacement date
	oldAgg, err = s.RunCommand(ctx, p.OldStudentID, &eda.Student_TruncateSponsorship{
		SponsorId:           p.SponsorID,
		StartDate:           record.StartDate,
		PaymentId:           p.PaymentID,
		NewEndDate:          newEndDate,
		ReplacedByStudentId: newAgg.GetID(),
		Reason:              p.Reason,
		Version:             oldAgg.Version,
	})
	if err != nil {
		return fmt.Errorf("failed to truncate sponsorship: %w", err)
	}

	// 2) transfer the remaining window to the new student
	transferErr := func() error {
		if s.beforeTransferHook != nil {
			if err := s.beforeTransferHook(); err != nil {
				return err
			}
		}
		var runErr error
		newAgg, runErr = s.RunCommand(ctx, p.NewStudentID, &eda.Student_UpdateSponsorship{
			SponsorId:         p.SponsorID,
			StartDate:         newEndDate,
			EndDate:           originalEnd,
			PaymentId:         p.PaymentID,
			PaymentAmount:     paymentAmount,
			ReplacedStudentId: oldAgg.GetID(),
			Version:           newAgg.Version,
		})
		return runErr
	}()

	if transferErr != nil {
		// Compensate: restore the original end date on the old record.
		if _, compErr := s.RunCommand(ctx, p.OldStudentID, &eda.Student_TruncateSponsorship{
			SponsorId:  p.SponsorID,
			StartDate:  record.StartDate,
			PaymentId:  p.PaymentID,
			NewEndDate: originalEnd,
			Reason:     fmt.Sprintf("compensation: transfer to student %d failed", p.NewStudentID),
			Version:    oldAgg.Version,
		}); compErr != nil {
			slog.Error("sponsorship replacement compensation FAILED; manual repair required",
				"old_student_id", p.OldStudentID,
				"new_student_id", p.NewStudentID,
				"sponsor_id", p.SponsorID,
				"original_end_date", originalEnd,
				"error", compErr,
			)
			return errors.Join(
				fmt.Errorf("transfer failed AND compensation failed; old student %d sponsorship is truncated but not transferred", p.OldStudentID),
				transferErr, compErr,
			)
		}
		return fmt.Errorf("failed to transfer sponsorship (old record restored): %w", transferErr)
	}

	// Refresh projections synchronously so the admin UI reflects the swap
	// immediately (the event-handler path is async).
	if err := s.repo.upsertSponsorshipProjections(oldAgg); err != nil {
		slog.Error("failed to refresh sponsorship projections for old student", "id", p.OldStudentID, "error", err)
	}
	if err := s.repo.upsertSponsorshipProjections(newAgg); err != nil {
		slog.Error("failed to refresh sponsorship projections for new student", "id", p.NewStudentID, "error", err)
	}

	return nil
}
```

Note on the compensation restore: the truncated record still matches by `PaymentId` (or sponsor+start, both unchanged by truncation), and the aggregate event permits re-extending because it only requires `new_end_date >= start_date`. The restore intentionally passes an empty `ReplacedByStudentId`, which clears the pointer.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/student/ -run TestReplaceSponsorship -v`
Expected: all PASS (7 test functions, subtests included).

- [ ] **Step 6: Run the full student package**

Run: `go test ./internal/student/ -count=1`
Expected: PASS (plus the pre-existing skipped integration test if dev.db is absent).

- [ ] **Step 7: Commit**

```bash
git add internal/student/sponsorship_replace.go internal/student/service.go internal/student/sponsorship_integration_test.go
git commit -m "Add ReplaceSponsorship service operation with validation and compensation"
```

---

### Task 6: Admin web — replace flow, report button, student-page history

**Files:**
- Create: `internal/webapi/admin_sponsorship.go`
- Create: `internal/webapi/templates/admin/sponsorship/replace.templ`
- Modify: `internal/webapi/api.go:248-259` (mount route in the /admin group)
- Modify: `internal/webapi/admin_reports.go:370-377` (pass PaymentID through)
- Modify: `internal/webapi/templates/admin/reports/sponsored_students.templ` (Actions column + struct field)
- Modify: `internal/webapi/templates/admin/student/admin_view_student.templ` (sponsorship history section)
- Test: `internal/webapi/admin_sponsorship_test.go`

**Interfaces:**
- Consumes: `svc.ReplaceSponsorship(ctx, student.ReplaceSponsorshipParams)` from Task 5; `svc.ListStudents` with `student.ActiveOnly()`, `student.EligibleForSponsorshipOnly()`, `student.WithNameSearch()`; `agg.FindSponsorshipRecord`.
- Produces: routes `GET/POST /admin/sponsorship/replace` (admin-only), UI entry points on the report and the student page.

- [ ] **Step 1: Write the failing auth test**

Create `internal/webapi/admin_sponsorship_test.go` (mirrors the pattern in `api_feeding_count_test.go`; `requireAdmin` returns 403 when no admin role is in the request context):

```go
package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestAdminSponsorshipRoutes_RejectNonAdmin(t *testing.T) {
	s := &Server{}

	r := chi.NewRouter()
	r.Route("/admin/sponsorship", func(r chi.Router) {
		r.Use(s.requireAdmin)
		s.sponsorshipAdminRoutes(r)
	})

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/admin/sponsorship/replace"},
		{http.MethodPost, "/admin/sponsorship/replace"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: expected 403 for non-admin, got %d", tc.method, tc.path, w.Code)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/webapi/ -run TestAdminSponsorshipRoutes_RejectNonAdmin -v`
Expected: compile error — `s.sponsorshipAdminRoutes` undefined.

- [ ] **Step 3: Create the templ page**

Create `internal/webapi/templates/admin/sponsorship/replace.templ`:

```go
package sponsorshiptempl

import (
	"fmt"
	"time"

	"geevly/internal/student"
)

type ReplacePickerParams struct {
	OldStudentID   uint64
	OldStudentName string
	SponsorID      string
	SponsorName    string
	StartDate      time.Time
	EndDate        time.Time
	PaymentID      string
	Candidates     []*student.ProjectedStudent
	Search         string
}

templ ReplaceSponsorshipPicker(p ReplacePickerParams) {
	<div class="container mx-auto px-4 py-8 max-w-3xl">
		<h1 class="text-2xl font-bold mb-2">Replace Sponsored Student</h1>
		<div class="bg-amber-50 border-l-4 border-amber-400 p-4 mb-6 text-sm text-amber-800">
			Replacing <strong>{ p.OldStudentName }</strong> on the sponsorship of
			<strong>
				if p.SponsorName != "" {
					{ p.SponsorName }
				} else {
					{ p.SponsorID }
				}
			</strong>
			({ p.StartDate.Format("2006-01-02") } → { p.EndDate.Format("2006-01-02") }).
			The current record will end today; the selected student will be sponsored from today through { p.EndDate.Format("2006-01-02") }.
		</div>
		<form method="GET" action="/admin/sponsorship/replace" class="mb-4 flex gap-2">
			<input type="hidden" name="oldStudentID" value={ fmt.Sprintf("%d", p.OldStudentID) }/>
			<input type="hidden" name="sponsorID" value={ p.SponsorID }/>
			<input type="hidden" name="startDate" value={ p.StartDate.Format("2006-01-02") }/>
			<input type="hidden" name="paymentID" value={ p.PaymentID }/>
			<input
				type="text"
				name="search"
				value={ p.Search }
				placeholder="Search eligible students by name…"
				class="flex-1 border border-gray-300 rounded-md px-3 py-2 text-sm"
			/>
			<button type="submit" class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50">Search</button>
		</form>
		<form method="POST" action="/admin/sponsorship/replace">
			<input type="hidden" name="old_student_id" value={ fmt.Sprintf("%d", p.OldStudentID) }/>
			<input type="hidden" name="sponsor_id" value={ p.SponsorID }/>
			<input type="hidden" name="start_date" value={ p.StartDate.Format("2006-01-02") }/>
			<input type="hidden" name="payment_id" value={ p.PaymentID }/>
			<div class="bg-white rounded-lg shadow overflow-hidden mb-4">
				<table class="w-full text-sm text-left text-gray-500">
					<thead class="text-xs text-gray-700 uppercase bg-gray-50">
						<tr>
							<th class="px-4 py-3"></th>
							<th class="px-4 py-3">Student</th>
							<th class="px-4 py-3">School ID</th>
							<th class="px-4 py-3">Grade</th>
						</tr>
					</thead>
					<tbody>
						for _, c := range p.Candidates {
							if uint64(c.ID) != p.OldStudentID {
								<tr class="border-b hover:bg-gray-50">
									<td class="px-4 py-3">
										<input type="radio" name="new_student_id" value={ fmt.Sprintf("%d", c.ID) } required/>
									</td>
									<td class="px-4 py-3 font-medium text-gray-900">{ c.FirstName } { c.LastName }</td>
									<td class="px-4 py-3">{ c.StudentID }</td>
									<td class="px-4 py-3">{ fmt.Sprintf("%d", c.Grade) }</td>
								</tr>
							}
						}
					</tbody>
				</table>
				if len(p.Candidates) == 0 {
					<p class="p-4 text-center text-gray-500">No eligible, available students found.</p>
				}
			</div>
			<div class="bg-white rounded-lg shadow p-4 mb-4">
				<label class="block text-sm font-medium text-gray-700 mb-1" for="reason">Reason for replacement (required)</label>
				<select name="reason_preset" class="border border-gray-300 rounded-md px-3 py-2 text-sm mb-2 w-full">
					<option value="Student left school">Student left school</option>
					<option value="Moved away">Moved away</option>
					<option value="Graduated">Graduated</option>
					<option value="">Other (describe below)</option>
				</select>
				<input
					type="text"
					name="reason_detail"
					placeholder="Additional detail (required if 'Other')"
					class="border border-gray-300 rounded-md px-3 py-2 text-sm w-full"
				/>
			</div>
			<div class="flex gap-2">
				<button type="submit" class="px-4 py-2 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700">Replace Student</button>
				<a href={ templ.SafeURL(fmt.Sprintf("/admin/student/%d", p.OldStudentID)) } class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50">Cancel</a>
			</div>
		</form>
	</div>
}
```

- [ ] **Step 4: Create the handlers**

Create `internal/webapi/admin_sponsorship.go`:

```go
package webapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"geevly/internal/student"
	sponsorshiptempl "geevly/internal/webapi/templates/admin/sponsorship"
	layouts "geevly/internal/webapi/templates/layouts"

	vex "github.com/Howard3/valueextractor"
	"github.com/go-chi/chi/v5"
)

func (s *Server) sponsorshipAdminRoutes(r chi.Router) {
	r.Get("/replace", s.adminReplaceSponsorshipForm)
	r.Post("/replace", s.adminReplaceSponsorship)
}

// adminReplaceSponsorshipForm renders the replacement picker for one
// sponsorship, identified by old student + sponsor + start date (+ optional
// payment id for post-fix records).
func (s *Server) adminReplaceSponsorshipForm(w http.ResponseWriter, r *http.Request) {
	ex := vex.Using(&vex.QueryExtractor{Query: r.URL.Query()}, vex.WithOptionalKeys("paymentID", "search"))
	oldStudentID := *vex.ReturnUint64(ex, "oldStudentID")
	sponsorID := *vex.ReturnString(ex, "sponsorID")
	startDateStr := *vex.ReturnString(ex, "startDate")
	paymentID := *vex.ReturnString(ex, "paymentID")
	search := *vex.ReturnString(ex, "search")
	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing parameters", ex.JoinedErrors())
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		s.errorPage(w, r, "Invalid start date", err)
		return
	}

	oldStudent, err := s.Services.StudentSvc.GetStudent(r.Context(), oldStudentID)
	if err != nil {
		s.errorPage(w, r, "Error loading student", err)
		return
	}

	record, err := oldStudent.FindSponsorshipRecord(paymentID, sponsorID, ProtoDateFromTime(startDate))
	if err != nil {
		s.errorPage(w, r, "Error locating sponsorship", err)
		return
	}

	opts := []student.ListOption{student.ActiveOnly(), student.EligibleForSponsorshipOnly()}
	if search != "" {
		opts = append(opts, student.WithNameSearch(search))
	}
	candidates, err := s.Services.StudentSvc.ListStudents(r.Context(), 50, 1, opts...)
	if err != nil {
		s.errorPage(w, r, "Error listing eligible students", err)
		return
	}

	params := sponsorshiptempl.ReplacePickerParams{
		OldStudentID:   oldStudentID,
		OldStudentName: oldStudent.GetFullName(),
		SponsorID:      sponsorID,
		StartDate:      startDate,
		EndDate: time.Date(
			int(record.EndDate.GetYear()), time.Month(record.EndDate.GetMonth()), int(record.EndDate.GetDay()),
			0, 0, 0, 0, time.UTC,
		),
		PaymentID:  paymentID,
		Candidates: candidates.Students,
		Search:     search,
	}

	// Enrich with the sponsor's display name; degrade gracefully to the raw ID.
	if clerkUser, err := s.SponsorClerk.Users().Read(sponsorID); err == nil {
		var parts []string
		if clerkUser.FirstName != nil && *clerkUser.FirstName != "" {
			parts = append(parts, *clerkUser.FirstName)
		}
		if clerkUser.LastName != nil && *clerkUser.LastName != "" {
			parts = append(parts, *clerkUser.LastName)
		}
		params.SponsorName = strings.Join(parts, " ")
	}

	s.renderTempl(w, r, sponsorshiptempl.ReplaceSponsorshipPicker(params))
}

func (s *Server) adminReplaceSponsorship(w http.ResponseWriter, r *http.Request) {
	ex := vex.Using(&vex.FormExtractor{Request: r}, vex.WithOptionalKeys("payment_id", "reason_preset", "reason_detail"))
	oldStudentID := *vex.ReturnUint64(ex, "old_student_id")
	newStudentID := *vex.ReturnUint64(ex, "new_student_id")
	sponsorID := *vex.ReturnString(ex, "sponsor_id")
	startDateStr := *vex.ReturnString(ex, "start_date")
	paymentID := *vex.ReturnString(ex, "payment_id")
	reasonPreset := *vex.ReturnString(ex, "reason_preset")
	reasonDetail := *vex.ReturnString(ex, "reason_detail")
	if err := ex.Errors(); err != nil {
		s.errorPage(w, r, "Error parsing form", ex.JoinedErrors())
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		s.errorPage(w, r, "Invalid start date", err)
		return
	}

	reason := strings.TrimSpace(strings.TrimSpace(reasonPreset) + " " + strings.TrimSpace(reasonDetail))

	err = s.Services.StudentSvc.ReplaceSponsorship(r.Context(), student.ReplaceSponsorshipParams{
		OldStudentID:    oldStudentID,
		NewStudentID:    newStudentID,
		SponsorID:       sponsorID,
		StartDate:       startDate,
		PaymentID:       paymentID,
		Reason:          reason,
		ReplacementDate: time.Now(),
	})
	if err != nil {
		s.errorPage(w, r, "Error replacing sponsored student", err)
		return
	}

	s.renderTempl(w, r, layouts.HTMXRedirect(fmt.Sprintf("/admin/student/%d", oldStudentID), "Sponsorship replaced"))
}
```

Add the `ProtoDateFromTime` helper to `internal/webapi/valueextractor.go` (below `ReturnProtoDate`):

```go
// ProtoDateFromTime converts a time.Time to an eda.Date.
func ProtoDateFromTime(t time.Time) *eda.Date {
	return &eda.Date{Year: int32(t.Year()), Month: int32(t.Month()), Day: int32(t.Day())}
}
```

(Add `"time"` to that file's imports if not present.)

- [ ] **Step 5: Mount the routes**

In `internal/webapi/api.go`, inside the `/admin` route block (after `r.Route("/reports", s.adminReports)`, line ~254):

```go
		r.Route("/sponsorship", s.sponsorshipAdminRoutes)
```

- [ ] **Step 6: Report entry point**

In `internal/webapi/templates/admin/reports/sponsored_students.templ`:

Add to the `SponsoredStudent` struct:

```go
type SponsoredStudent struct {
	StudentID   string
	StudentName string
	SponsorID   string
	StartDate   time.Time
	EndDate     time.Time
	PaymentID   string
}
```

Add an Actions column header after the End Date `<th>`:

```html
									<th scope="col" class="px-6 py-3">Actions</th>
```

Add the action cell after the End Date `<td>` in the student row loop:

```html
										<td class="px-6 py-4">
											<a
												href={ templ.SafeURL(fmt.Sprintf("/admin/sponsorship/replace?oldStudentID=%s&sponsorID=%s&startDate=%s&paymentID=%s", student.StudentID, student.SponsorID, student.StartDate.Format("2006-01-02"), student.PaymentID)) }
												class="inline-flex items-center px-3 py-1.5 text-xs font-medium text-amber-700 bg-white border border-amber-300 rounded-md hover:bg-amber-50"
											>
												Replace Student
											</a>
										</td>
```

In `internal/webapi/admin_reports.go` `adminSponsoredStudentsReport` (line ~370), add `PaymentID` to the struct literal:

```go
		sponsoredStudent := reportstempl.SponsoredStudent{
			StudentID:   sp.StudentID,
			StudentName: fmt.Sprintf("%s %s", student.GetStudent().FirstName, student.GetStudent().LastName),
			SponsorID:   sp.SponsorID,
			StartDate:   sp.StartDate,
			EndDate:     sp.EndDate,
			PaymentID:   sp.PaymentID,
		}
```

- [ ] **Step 7: Student-page entry point**

In `internal/webapi/templates/admin/student/admin_view_student.templ`:

Add `"time"` to the imports if absent. Add these helpers near `dateToFormDate` (line ~295):

```go
func protoDateString(d *eda.Date) string {
	return fmt.Sprintf("%04d-%02d-%02d", d.GetYear(), d.GetMonth(), d.GetDay())
}

func sponsorshipIsCurrent(rec *eda.Student_SponsorshipRecord) bool {
	end := time.Date(int(rec.EndDate.GetYear()), time.Month(rec.EndDate.GetMonth()), int(rec.EndDate.GetDay()), 23, 59, 59, 0, time.UTC)
	return time.Now().UTC().Before(end)
}
```

Add a new section component:

```go
templ sponsorshipHistorySection(params ViewParams) {
	if len(params.Student.SponsorshipHistory) > 0 {
		<div class="bg-white rounded-lg shadow p-6 mt-6">
			<h3 class="text-lg font-semibold text-gray-900 mb-4">Sponsorships</h3>
			<div class="overflow-x-auto">
				<table class="w-full text-sm text-left text-gray-500">
					<thead class="text-xs text-gray-700 uppercase bg-gray-50">
						<tr>
							<th class="px-4 py-2">Sponsor</th>
							<th class="px-4 py-2">Start</th>
							<th class="px-4 py-2">End</th>
							<th class="px-4 py-2">Notes</th>
							<th class="px-4 py-2">Actions</th>
						</tr>
					</thead>
					<tbody>
						for _, rec := range params.Student.SponsorshipHistory {
							<tr class="border-b">
								<td class="px-4 py-2">{ rec.SponsorId }</td>
								<td class="px-4 py-2">{ protoDateString(rec.StartDate) }</td>
								<td class="px-4 py-2">{ protoDateString(rec.EndDate) }</td>
								<td class="px-4 py-2 text-xs">
									if rec.ReplacedByStudentId != "" {
										<span class="text-amber-700">
											Replaced by
											<a class="underline" href={ templ.SafeURL(fmt.Sprintf("/admin/student/%s", rec.ReplacedByStudentId)) }>student { rec.ReplacedByStudentId }</a>
											on { protoDateString(rec.EndDate) }
											if rec.Reason != "" {
												— { rec.Reason }
											}
										</span>
									} else if rec.ReplacedStudentId != "" {
										<span class="text-green-700">
											Continued from
											<a class="underline" href={ templ.SafeURL(fmt.Sprintf("/admin/student/%s", rec.ReplacedStudentId)) }>student { rec.ReplacedStudentId }</a>
											on { protoDateString(rec.StartDate) }
										</span>
									}
								</td>
								<td class="px-4 py-2">
									if sponsorshipIsCurrent(rec) && rec.ReplacedByStudentId == "" && !params.Student.IsDeleted {
										<a
											href={ templ.SafeURL(fmt.Sprintf("/admin/sponsorship/replace?oldStudentID=%d&sponsorID=%s&startDate=%s&paymentID=%s", params.ID, rec.SponsorId, protoDateString(rec.StartDate), rec.PaymentId)) }
											class="inline-flex items-center px-2 py-1 text-xs font-medium text-amber-700 bg-white border border-amber-300 rounded-md hover:bg-amber-50"
										>
											Replace Student
										</a>
									}
								</td>
							</tr>
						}
					</tbody>
				</table>
			</div>
		</div>
	}
}
```

Call it from `AdminViewStudent` directly after the `@statusSection(...)` call (line ~144):

```go
				@sponsorshipHistorySection(params)
```

- [ ] **Step 8: Regenerate templates and build**

Run: `go tool templ generate && go build ./...`
Expected: templ reports generated files (including `replace_templ.go`, updated `sponsored_students_templ.go`, `admin_view_student_templ.go`); build exits 0.

- [ ] **Step 9: Run the auth test and package tests**

Run: `go test ./internal/webapi/ -count=1 -v -run TestAdminSponsorship`
Expected: PASS — both routes return 403 without an admin role.

- [ ] **Step 10: Commit**

```bash
git add internal/webapi/admin_sponsorship.go internal/webapi/admin_sponsorship_test.go internal/webapi/api.go internal/webapi/admin_reports.go internal/webapi/valueextractor.go internal/webapi/templates/
git commit -m "Add admin sponsorship replacement flow: picker, report and student-page entry points"
```

---

### Task 7: Final verification

**Files:** none new.

- [ ] **Step 1: Full build + vet + tests**

Run: `go vet ./... && go build ./... && go test ./internal/student/ ./internal/webapi/ -count=1`
Expected: everything passes. Fix anything that fails before proceeding.

- [ ] **Step 2: Manual smoke test (if a dev DB is available)**

Run `task dev`, then as an admin:
1. Open `/admin/reports/sponsored-students` → each row shows a "Replace Student" button.
2. Click it → picker lists only eligible+available students, shows the sponsor and window.
3. Pick a student, choose a reason, submit → redirected to the old student's page; its Sponsorships section shows the truncated record with "Replaced by … — reason"; the new student's page shows "Continued from …".
4. Re-open the report → the sponsor now shows the new student with today → original end date.

- [ ] **Step 3: Verify commit history is clean**

Run: `git log --oneline master..HEAD` (or the feature branch base)
Expected: one commit per task, no `Co-Authored-By: Claude` trailers (`git log --format=%B | grep -i co-authored` returns nothing).
