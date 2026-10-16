package deploy

import (
	"strings"
	"testing"
	"time"
)

// fixClock pins the server-day clock for the test, away from any test
// offset's midnight.
func fixClock(t *testing.T) {
	t.Helper()
	fixed := time.Date(2026, 10, 16, 13, 17, 0, 0, time.UTC)
	previous := watchNow
	watchNow = func() time.Time { return fixed }
	t.Cleanup(func() { watchNow = previous })
}

// The day after a busy log day: the default newest-entry read covers
// yesterday's oversized files and is refused, a read from the server's
// midnight is not. The watch arms on the process id as usual.
func TestWatchProbeReadsTheServerDayWhenYesterdayIsOversized(t *testing.T) {
	fixClock(t)
	for _, offset := range []string{"00:00:00", "-05:00:00", "05:30:00", "14:00:00", "-12:00:00"} {
		t.Run(offset, func(t *testing.T) {
			env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, yesterdayOversized: true, serverOffset: offset}
			lines, err := runWatch(t, env, "--timeout", "100ms")
			if exitCode(err) != 6 {
				t.Fatalf("expected a timeout (exit 6), got %v", err)
			}
			baseline := detailOf(phaseLine(lines, "baseline"))
			if baseline["processId"] != "111" || baseline["landingBasis"] != "process-id" {
				t.Fatalf("the probe should read the process id from the server's day: %v", baseline)
			}
			// The default read is tried once; after the same-day read works the
			// probe keeps using it instead of asking for yesterday every poll.
			unbounded := 0
			for _, start := range env.newestStarts {
				if start == "" {
					unbounded++
				}
			}
			if unbounded != 1 || len(env.newestStarts) < 4 {
				t.Fatalf("expected one default read, then same-day reads: %v", env.newestStarts)
			}
		})
	}
}

func TestWatchLandsOnTheProcessIDWhenYesterdayIsOversized(t *testing.T) {
	fixClock(t)
	env := &fakeDeployEnv{downTicks: map[int]bool{2: true, 3: true}, landTick: 4, yesterdayOversized: true, serverOffset: "-05:00:00"}
	lines, err := runWatch(t, env)
	if err != nil {
		t.Fatalf("watch failed: %v\n%v", err, phasesOf(lines))
	}
	landed := detailOf(phaseLine(lines, "landed"))
	if landed["basis"] != "process-id" || landed["processId"] != "111 → 222" {
		t.Fatalf("expected a process-id landing: %v", landed)
	}
}

// Without the server's offset (no /server/information) the probe cannot
// tell the server's day, so it keeps the refused read and falls back to the
// restart-cycle basis as before.
func TestWatchProbeWithoutTheServerOffsetFallsBackAsBefore(t *testing.T) {
	fixClock(t)
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, yesterdayOversized: true}
	lines, err := runWatch(t, env, "--timeout", "100ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v", err)
	}
	if baseline := detailOf(phaseLine(lines, "baseline")); baseline["landingBasis"] != "restart-cycle" {
		t.Fatalf("expected the restart-cycle basis without the server's offset: %v", baseline)
	}
}

// A same-day read that is refused too (today's files are oversized) also
// falls back as before.
func TestWatchProbeFallsBackWhenTheServerDayIsRefusedToo(t *testing.T) {
	fixClock(t)
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, refuseLogs: true, serverOffset: "00:00:00"}
	lines, err := runWatch(t, env, "--timeout", "100ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v", err)
	}
	if baseline := detailOf(phaseLine(lines, "baseline")); baseline["landingBasis"] != "restart-cycle" {
		t.Fatalf("expected the restart-cycle basis: %v", baseline)
	}
	for _, start := range env.newestStarts {
		if start != "" && !strings.HasSuffix(start, "T00:00:00Z") {
			t.Fatalf("a UTC server's day starts at UTC midnight, got %s", start)
		}
	}
}

func TestServerDayStartFollowsTheServerOffset(t *testing.T) {
	for _, tc := range []struct {
		now    string
		offset string
		want   string
	}{
		{"2026-10-16T07:55:00Z", "00:00:00", "2026-10-16T00:00:00Z"},
		// 22:00 on the 15th in UTC-5: the server's day is still the 15th.
		{"2026-10-16T03:00:00Z", "-05:00:00", "2026-10-15T05:00:00Z"},
		// 01:30 on the 16th in UTC+5:30: the server's day is already the 16th.
		{"2026-10-15T20:00:00Z", "05:30:00", "2026-10-15T18:30:00Z"},
		{"2026-10-15T09:59:59Z", "14:00:00", "2026-10-14T10:00:00Z"},
		{"2026-10-15T10:00:00Z", "14:00:00", "2026-10-15T10:00:00Z"},
		{"2026-10-16T11:59:59Z", "-12:00:00", "2026-10-15T12:00:00Z"},
	} {
		now, _ := time.Parse(time.RFC3339, tc.now)
		offset, err := parseUtcOffset(tc.offset)
		if err != nil {
			t.Fatalf("%s: %v", tc.offset, err)
		}
		if got := serverDayStart(now, offset).UTC().Format(time.RFC3339); got != tc.want {
			t.Fatalf("now %s, offset %s: got %s, want %s", tc.now, tc.offset, got, tc.want)
		}
	}
}

func TestParseUtcOffsetReadsTimeSpans(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"00:00:00":  0,
		"01:00:00":  time.Hour,
		"-05:00:00": -5 * time.Hour,
		"05:30:00":  5*time.Hour + 30*time.Minute,
		"-03:30:00": -(3*time.Hour + 30*time.Minute),
	} {
		got, err := parseUtcOffset(value)
		if err != nil || got != want {
			t.Fatalf("%s: got %s (%v), want %s", value, got, err, want)
		}
	}
	for _, value := range []string{"", "5", "1.00:00:00", "aa:bb:cc", "15:00:00", "01:60:00"} {
		if _, err := parseUtcOffset(value); err == nil {
			t.Fatalf("%q must not parse", value)
		}
	}
}

// Just after the server's midnight the same-day read has no entries yet:
// the probe keeps the refusal, so the watch still arms on the restart-cycle
// basis instead of refusing to arm for a missing ProcessId.
func TestWatchProbeWithNoEntriesYetTodayFallsBack(t *testing.T) {
	fixClock(t)
	env := &fakeDeployEnv{downTicks: map[int]bool{}, landTick: 1 << 30, yesterdayOversized: true, todayEmpty: true, serverOffset: "00:00:00"}
	lines, err := runWatch(t, env, "--timeout", "100ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v", err)
	}
	if baseline := detailOf(phaseLine(lines, "baseline")); baseline["landingBasis"] != "restart-cycle" {
		t.Fatalf("expected the restart-cycle basis: %v", baseline)
	}
}
