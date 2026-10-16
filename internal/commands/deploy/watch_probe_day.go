package deploy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/jsonvalue"
)

// Umbraco 18.2's log viewer sizes a read by the daily log files of every
// server-local day from startDate to now, and an unset startDate means a day
// ago. So the day after a busy log day, the default newest-entry read is
// refused while one starting at today's server midnight is not. The probe
// keeps the default read and, only when that is refused for size, reads
// from the start of the server's day.
//
// The server's day comes from /server/information's baseUtcOffset, which is
// the time zone's standard offset (TimeZoneInfo.BaseUtcOffset). Daylight
// saving time moves the server's clock ahead of it, so the computed midnight
// falls at 01:00 local at the latest: still the same day, never yesterday.
// In the hour after a DST midnight the computed day is the previous one, the
// read is refused again, and the probe falls back as before.

// watchProbeDay is the state the probes share across the baseline and the
// poll loop: the server's offset, read once, and whether the same-day read
// is the one that works.
type watchProbeDay struct {
	mu      sync.Mutex
	fetched bool
	offset  time.Duration
	known   bool
	sameDay bool
}

func (d *watchProbeDay) useSameDay() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sameDay
}

func (d *watchProbeDay) setSameDay(on bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sameDay = on
}

// serverOffset reads /server/information once per watch. A failure or an
// unreadable value leaves the offset unknown, and the probe keeps the
// default read.
func (d *watchProbeDay) serverOffset(ctx context.Context, client *api.Client) (time.Duration, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fetched {
		return d.offset, d.known
	}
	d.fetched = true
	result, err := client.Get(ctx, "/server/information", api.RequestOptions{})
	if err != nil {
		return 0, false
	}
	info, ok := result.(map[string]any)
	if !ok {
		return 0, false
	}
	offset, err := parseUtcOffset(jsonvalue.String(info["baseUtcOffset"]))
	if err != nil {
		return 0, false
	}
	d.offset, d.known = offset, true
	return offset, true
}

// parseUtcOffset reads a .NET TimeSpan as /server/information prints it:
// "00:00:00", "-05:00:00", "05:30:00".
func parseUtcOffset(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	sign := time.Duration(1)
	if strings.HasPrefix(value, "-") {
		sign, value = -1, value[1:]
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("unexpected baseUtcOffset %q", value)
	}
	hours, err1 := strconv.Atoi(parts[0])
	minutes, err2 := strconv.Atoi(parts[1])
	seconds, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || hours > 14 || minutes > 59 || seconds > 59 {
		return 0, fmt.Errorf("unexpected baseUtcOffset %q", value)
	}
	return sign * (time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second), nil
}

// serverDayStart is the instant of the server's midnight for now, given
// the server's offset from UTC.
func serverDayStart(now time.Time, offset time.Duration) time.Time {
	local := now.UTC().Add(offset)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	return midnight.Add(-offset)
}
