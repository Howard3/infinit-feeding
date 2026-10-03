package student

import (
	"time"

	_ "time/tzdata"
)

// FeedingTimeZone is the calendar used for one-feed-per-day until school
// timezones are derived from city. All current schools are in the Philippines.
const FeedingTimeZone = "Asia/Manila"

var feedingLocation = mustLoadFeedingLocation()

func mustLoadFeedingLocation() *time.Location {
	loc, err := time.LoadLocation(FeedingTimeZone)
	if err != nil {
		panic("load feeding timezone " + FeedingTimeZone + ": " + err.Error())
	}
	return loc
}

// FeedingLocation is Asia/Manila. Use this for fed-today windows in the UI.
func FeedingLocation() *time.Location {
	return feedingLocation
}

// StartOfFeedingDay is midnight in Asia/Manila for the calendar day containing t.
func StartOfFeedingDay(t time.Time) time.Time {
	local := t.In(feedingLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, feedingLocation)
}

// FeedingCalendarDate returns YYYY-MM-DD for the school timezone (Asia/Manila)
// at the given unix instant. Never take this from the client.
func FeedingCalendarDate(unixTimestamp uint64) string {
	return time.Unix(int64(unixTimestamp), 0).In(feedingLocation).Format("2006-01-02")
}

// StartOfFeedingCalendarDate is midnight Asia/Manila for a YYYY-MM-DD calendar
// date string (or a time whose Year/Month/Day were parsed from one).
func StartOfFeedingCalendarDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, feedingLocation)
}
