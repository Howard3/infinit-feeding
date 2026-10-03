package student

import (
	"testing"
	"time"

	"geevly/gen/go/eda"
)

func manilaTZ(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Manila")
	if err != nil {
		t.Fatalf("load Asia/Manila: %v", err)
	}
	return loc
}

func TestFeedingCalendarDateUsesManilaMidnight(t *testing.T) {
	manila := manilaTZ(t)

	// 6:00 AM PHT is 22:00 UTC the previous day. UTC midnight is 8:00 AM PHT.
	tue6am := time.Date(2026, 9, 8, 6, 0, 0, 0, manila)
	got := FeedingCalendarDate(uint64(tue6am.Unix()))
	if got != "2026-09-08" {
		t.Fatalf("6 AM PHT should be Manila date 2026-09-08, got %s", got)
	}

	mon22utc := time.Date(2026, 9, 7, 22, 0, 0, 0, time.UTC)
	got = FeedingCalendarDate(uint64(mon22utc.Unix()))
	if got != "2026-09-08" {
		t.Fatalf("22:00 UTC should already be the next Manila date, got %s", got)
	}

	justBeforeMidnight := time.Date(2026, 9, 7, 23, 59, 0, 0, manila)
	got = FeedingCalendarDate(uint64(justBeforeMidnight.Unix()))
	if got != "2026-09-07" {
		t.Fatalf("23:59 PHT should still be 2026-09-07, got %s", got)
	}
}

func TestFeedAllows6AMManilaAfterPreviousAfternoon(t *testing.T) {
	manila := manilaTZ(t)
	agg := &Aggregate{data: &eda.Student{}}

	mondayLunch := time.Date(2026, 9, 7, 12, 0, 0, 0, manila).Unix()
	tuesday6am := time.Date(2026, 9, 8, 6, 0, 0, 0, manila).Unix()

	if _, err := agg.Feed(&eda.Student_Feeding{UnixTimestamp: uint64(mondayLunch), Version: 0}); err != nil {
		t.Fatalf("Monday lunch feed: %v", err)
	}
	if _, err := agg.Feed(&eda.Student_Feeding{UnixTimestamp: uint64(tuesday6am), Version: agg.Version}); err != nil {
		t.Fatalf("Tuesday 6 AM PHT must be a new feeding day, got: %v", err)
	}
}

func TestFeedSameDayUsesFullCalendarDate(t *testing.T) {
	agg := &Aggregate{data: &eda.Student{}}

	jan5 := time.Date(2026, 1, 5, 10, 0, 0, 0, manilaTZ(t)).Unix()
	feb5 := time.Date(2026, 2, 5, 10, 0, 0, 0, manilaTZ(t)).Unix()

	if _, err := agg.Feed(&eda.Student_Feeding{UnixTimestamp: uint64(jan5), Version: 0}); err != nil {
		t.Fatalf("first feed: %v", err)
	}
	if _, err := agg.Feed(&eda.Student_Feeding{UnixTimestamp: uint64(feb5), Version: agg.Version}); err != nil {
		t.Fatalf("Feb 5 must not collide with Jan 5 (old .Day()-only bug): %v", err)
	}
}

func TestFeedRejectsSameManilaCalendarDay(t *testing.T) {
	agg := &Aggregate{data: &eda.Student{}}

	morning := time.Date(2026, 3, 10, 8, 0, 0, 0, manilaTZ(t)).Unix()
	evening := time.Date(2026, 3, 10, 18, 0, 0, 0, manilaTZ(t)).Unix()

	if _, err := agg.Feed(&eda.Student_Feeding{
		UnixTimestamp: uint64(morning),
		Version:       0,
	}); err != nil {
		t.Fatalf("first feed: %v", err)
	}
	if _, err := agg.Feed(&eda.Student_Feeding{
		UnixTimestamp: uint64(evening),
		Version:       agg.Version,
	}); err == nil {
		t.Fatal("expected same Manila calendar day rejection")
	}
}

func TestFeedIgnoresClientLocalCalendarDate(t *testing.T) {
	manila := manilaTZ(t)
	agg := &Aggregate{data: &eda.Student{}}

	morning := time.Date(2026, 3, 10, 8, 0, 0, 0, manila).Unix()
	evening := time.Date(2026, 3, 10, 18, 0, 0, 0, manila).Unix()

	if _, err := agg.Feed(&eda.Student_Feeding{
		UnixTimestamp:     uint64(morning),
		LocalCalendarDate: "2099-01-01", // client lie; ignored
		Version:           0,
	}); err != nil {
		t.Fatalf("first feed: %v", err)
	}

	got := agg.data.FeedingReport[0].LocalCalendarDate
	if got != "2026-03-10" {
		t.Fatalf("persisted date must be derived from unix, got %s", got)
	}

	// Client claims a different day; unix is still same Manila day → reject.
	if _, err := agg.Feed(&eda.Student_Feeding{
		UnixTimestamp:     uint64(evening),
		LocalCalendarDate: "2026-03-11",
		Version:           agg.Version,
	}); err == nil {
		t.Fatal("client local_calendar_date must not create a second meal on the same Manila day")
	}
}

func TestWasFedOnDayUsesManilaCalendar(t *testing.T) {
	manila := manilaTZ(t)
	// 6 AM PHT on Sep 8 is still Sep 7 UTC.
	fedAt := time.Date(2026, 9, 8, 6, 0, 0, 0, manila)
	g := &GroupedByStudentReturn{
		FeedingEvents: []ProjectedFeedingEvent{{FeedingDateTime: fedAt.UTC()}},
	}
	day := StartOfFeedingCalendarDate(2026, 9, 8)
	if !g.WasFedOnDay(day) {
		t.Fatal("6 AM PHT must count as fed on Manila Sep 8")
	}
	prev := StartOfFeedingCalendarDate(2026, 9, 7)
	if g.WasFedOnDay(prev) {
		t.Fatal("6 AM PHT Sep 8 must not count as Sep 7")
	}
}


func TestFeedStoresAttributionMetadata(t *testing.T) {
	agg := &Aggregate{data: &eda.Student{}}
	manila := manilaTZ(t)
	ts := time.Date(2026, 8, 4, 10, 30, 0, 0, manila).Unix()
	wantDate := FeedingCalendarDate(uint64(ts))

	if _, err := agg.Feed(&eda.Student_Feeding{
		UnixTimestamp:       uint64(ts),
		FileId:              "file-1",
		LocalCalendarDate:   "1999-01-01", // ignored
		RecordedByFeederId:  "user-a",
		SubmittedByFeederId: "user-b",
		DeviceId:            "device-1",
		ClientFeedId:        "client-1",
		SchoolId:            "42",
		Version:             0,
	}); err != nil {
		t.Fatalf("feed: %v", err)
	}

	got := agg.data.FeedingReport[0]
	if got.RecordedByFeederId != "user-a" || got.SubmittedByFeederId != "user-b" {
		t.Fatalf("attribution not stored: %+v", got)
	}
	if got.LocalCalendarDate != wantDate || got.DeviceId != "device-1" {
		t.Fatalf("metadata not stored: %+v (want date %s)", got, wantDate)
	}
}
