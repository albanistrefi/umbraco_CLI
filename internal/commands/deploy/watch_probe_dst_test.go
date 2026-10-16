package deploy

import (
	"testing"
	"time"
	_ "time/tzdata"
)

// standardOffset is a zone's offset outside daylight saving time, what
// /server/information's baseUtcOffset (TimeZoneInfo.BaseUtcOffset)
// reports. In these zones daylight saving time only ever adds to it.
func standardOffset(zone *time.Location, year int) time.Duration {
	_, january := time.Date(year, 1, 15, 12, 0, 0, 0, zone).Zone()
	_, july := time.Date(year, 7, 15, 12, 0, 0, 0, zone).Zone()
	return time.Duration(min(january, july)) * time.Second
}

func dateOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Umbraco 18.2 takes startDate.LocalDateTime.Date: the start converted to
// the server's own clock, daylight saving time included. Through a whole
// year, the same-day start must never lie in the future and must never
// cover the server's yesterday, except in the hour after a DST midnight,
// where the day computed from the standard offset is still yesterday and
// the read is expected to be refused (the probe then falls back).
func TestServerDayStartAcrossDaylightSavingTime(t *testing.T) {
	for _, name := range []string{"Europe/Copenhagen", "America/New_York", "Australia/Sydney", "Australia/Lord_Howe", "UTC"} {
		zone, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		base := standardOffset(zone, 2026)
		fallbacks := 0
		for now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); now.Year() == 2026; now = now.Add(15 * time.Minute) {
			start := serverDayStart(now, base)
			if start.After(now) {
				t.Fatalf("%s at %s: start %s is in the future", name, now, start)
			}
			serverToday := dateOf(now.In(zone))
			startDay := dateOf(start.In(zone))
			computedToday := dateOf(now.UTC().Add(base))
			switch {
			case startDay.Equal(serverToday):
			case startDay.Equal(serverToday.AddDate(0, 0, -1)) && !computedToday.Equal(serverToday):
				// Only while the server's clock is within the DST shift
				// after its midnight.
				local := now.In(zone)
				_, actual := local.Zone()
				shift := time.Duration(actual)*time.Second - base
				sinceMidnight := local.Sub(time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone))
				if shift <= 0 || sinceMidnight >= shift {
					t.Fatalf("%s at %s (server %s): falls back outside the DST shift after midnight", name, now, local)
				}
				fallbacks++
			default:
				t.Fatalf("%s at %s (server %s): start %s reads from %s", name, now, now.In(zone), start.In(zone), startDay.Format("2006-01-02"))
			}
		}
		if name == "UTC" && fallbacks != 0 {
			t.Fatalf("UTC never falls back, got %d", fallbacks)
		}
	}
}
