package deploy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/jsonvalue"
)

// Umbraco 18.2's log viewer sizes a read by the daily log files of every
// server-local day from startDate to now, and an unset startDate means a day
// ago. 18.2 also filters the entries by timestamp from startDate on, so a
// same-day read returns the newest entry since the server's midnight. So the day after a busy log day, the default newest-entry read is
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

// watchNow is the clock of the server-day computation; tests fix it.
var watchNow = time.Now

// watchProbeDay is the state the probes share across the baseline and the
// poll loop: the server's offset, and whether the same-day read is the one
// that works.
type watchProbeDay struct {
	mu sync.Mutex
	// settled is set once the offset is known or unreadable for good; a
	// failed request leaves it unset, so the next refusal asks again.
	settled bool
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

// serverOffset reads /server/information's baseUtcOffset. A value that
// reads, or one that never will (a 4xx answer, a missing or malformed
// field), settles it for the watch; a failed request (5xx, unreachable,
// timed out) does not, so the next refusal asks again.
// Unknown for good, the probe keeps the default read; a failed request is
// returned as an error.
func (d *watchProbeDay) serverOffset(ctx context.Context, client *api.Client) (time.Duration, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.settled {
		return d.offset, d.known, nil
	}
	result, err := client.Get(ctx, "/server/information", api.RequestOptions{})
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode < http.StatusInternalServerError {
		// The server answered and has no offset for us (no such route, no
		// permission): that will not change during the watch.
		d.settled = true
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	d.settled = true
	info, ok := result.(map[string]any)
	if !ok {
		return 0, false, nil
	}
	offset, err := parseUtcOffset(jsonvalue.String(info["baseUtcOffset"]))
	if err != nil {
		return 0, false, nil
	}
	d.offset, d.known = offset, true
	return offset, true, nil
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
