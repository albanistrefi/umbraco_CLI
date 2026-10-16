package deploy

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdtest"
)

// The day after a busy log day, same-day mode on, the process never
// changes, and an outage with no restart: one 500 on the same-day read at
// the first alive poll must not count as a size refusal and land the watch.
func TestWatchATransientSameDayFailureIsNotASizeRefusal(t *testing.T) {
	fixClock(t)
	env := &fakeDeployEnv{downTicks: map[int]bool{2: true, 3: true}, landTick: 1 << 30, yesterdayOversized: true,
		serverOffset: "00:00:00", sameDayFails: map[int]bool{4: true}}
	lines, err := runWatch(t, env, "--restart-min-down", "0", "--timeout", "300ms")
	if exitCode(err) != 6 {
		t.Fatalf("expected a timeout (exit 6), got %v\n%v", err, phasesOf(lines))
	}
	assertNotLanded(t, lines)
}

// probeScript answers the landing probe's requests: the default read (no
// startDate), the same-day read, and /server/information, in order per
// kind, repeating the last answer. It records the order of requests.
type probeScript struct {
	mu       sync.Mutex
	def      []func() *http.Response
	same     []func() *http.Response
	info     []func() *http.Response
	delay    time.Duration
	requests []string
}

func (s *probeScript) next(kind string, list *[]func() *http.Response) *http.Response {
	s.requests = append(s.requests, kind)
	answer := (*list)[0]
	if len(*list) > 1 {
		*list = (*list)[1:]
	}
	return answer()
}

func (s *probeScript) probes(t *testing.T, timeout time.Duration) *watchProbes {
	t.Helper()
	deps := cmdtest.Deps(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == cmdtest.TokenPath {
			return cmdtest.JSONResponse(http.StatusOK, `{"access_token":"token-123","expires_in":3600}`), nil
		}
		if s.delay > 0 {
			select {
			case <-time.After(s.delay):
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case req.URL.Path == "/umbraco/management/api/v1/server/information":
			return s.next("info", &s.info), nil
		case req.URL.Path == "/umbraco/management/api/v1/log-viewer/log" && req.URL.Query().Get("startDate") == "":
			return s.next("default", &s.def), nil
		case req.URL.Path == "/umbraco/management/api/v1/log-viewer/log":
			return s.next("same-day", &s.same), nil
		}
		return cmdtest.JSONResponse(http.StatusNotFound, `{}`), nil
	})
	return &watchProbes{client: deps.Client, timeout: timeout, day: &watchProbeDay{}}
}

func (s *probeScript) took() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := strings.Join(s.requests, ",")
	s.requests = nil
	return out
}

func refused() *http.Response {
	return cmdtest.JSONResponse(http.StatusBadRequest, logSizeRefusalBody)
}
func failed() *http.Response {
	return cmdtest.JSONResponse(http.StatusInternalServerError, `{"title":"boom"}`)
}
func empty() *http.Response { return cmdtest.JSONResponse(http.StatusOK, `{"total":0,"items":[]}`) }
func process(id string) func() *http.Response {
	return func() *http.Response {
		return cmdtest.JSONResponse(http.StatusOK, `{"total":1,"items":[{"timestamp":"2026-10-16T09:00:00Z","properties":[{"name":"ProcessId","value":"`+id+`"},{"name":"MachineName","value":"m1"}]}]}`)
	}
}
func offset() *http.Response {
	return cmdtest.JSONResponse(http.StatusOK, `{"baseUtcOffset":"00:00:00"}`)
}

func newest(t *testing.T, p *watchProbes) (string, error) {
	t.Helper()
	id, _, _, err := p.newestProcess(context.Background())
	return id, err
}

func TestTheProbeRetriesOnlyAfterASizeRefusal(t *testing.T) {
	fixClock(t)
	script := &probeScript{def: []func() *http.Response{failed}, same: []func() *http.Response{process("111")}, info: []func() *http.Response{offset}}
	p := script.probes(t, time.Second)
	if _, err := newest(t, p); err == nil || isLogSizeRefusal(err) {
		t.Fatalf("a 500 is returned as itself, got %v", err)
	}
	if got := script.took(); got != "default" {
		t.Fatalf("a 500 must not trigger the same-day read: %s", got)
	}
}

func TestSameDayModeIsLeftWhenTodayIsRefusedToo(t *testing.T) {
	fixClock(t)
	script := &probeScript{
		def:  []func() *http.Response{refused},
		same: []func() *http.Response{process("111"), refused},
		info: []func() *http.Response{offset},
	}
	p := script.probes(t, time.Second)
	if id, err := newest(t, p); err != nil || id != "111" {
		t.Fatalf("first read: %q %v", id, err)
	}
	if got := script.took(); got != "default,info,same-day" {
		t.Fatalf("first read requests: %s", got)
	}
	if _, err := newest(t, p); !isLogSizeRefusal(err) {
		t.Fatalf("today refused too is a size refusal, got %v", err)
	}
	if got := script.took(); got != "same-day,default,same-day" {
		t.Fatalf("second read requests: %s", got)
	}
	_, _ = newest(t, p)
	if got := script.took(); !strings.HasPrefix(got, "default") {
		t.Fatalf("after leaving same-day mode the default read comes first: %s", got)
	}
}

// After the server's midnight, same-day mode reads an empty new day: that
// is no process id, so the probe goes back to the default read and reports
// the refusal rather than an empty success.
func TestAnEmptyNewDayInSameDayModeIsNotASuccess(t *testing.T) {
	fixClock(t)
	script := &probeScript{
		def:  []func() *http.Response{refused},
		same: []func() *http.Response{process("111"), empty},
		info: []func() *http.Response{offset},
	}
	p := script.probes(t, time.Second)
	_, _ = newest(t, p)
	script.took()
	id, err := newest(t, p)
	if id != "" || !isLogSizeRefusal(err) {
		t.Fatalf("an empty new day reports the refusal, got %q %v", id, err)
	}
	if got := script.took(); got != "same-day,default,same-day" {
		t.Fatalf("requests: %s", got)
	}
}

func TestATransientSameDayFailureKeepsTheModeAndIsReturned(t *testing.T) {
	fixClock(t)
	script := &probeScript{
		def:  []func() *http.Response{refused},
		same: []func() *http.Response{process("111"), failed, process("111")},
		info: []func() *http.Response{offset},
	}
	p := script.probes(t, time.Second)
	_, _ = newest(t, p)
	script.took()
	if _, err := newest(t, p); err == nil || isLogSizeRefusal(err) {
		t.Fatalf("a same-day 500 is returned as itself, got %v", err)
	}
	if got := script.took(); got != "same-day" {
		t.Fatalf("a transient failure costs one request: %s", got)
	}
	if id, err := newest(t, p); err != nil || id != "111" {
		t.Fatalf("the mode stays on: %q %v", id, err)
	}
	if got := script.took(); got != "same-day" {
		t.Fatalf("the next read goes straight to the same day: %s", got)
	}
}

func TestATransientSameDayFailureAfterARefusalIsReturned(t *testing.T) {
	fixClock(t)
	script := &probeScript{def: []func() *http.Response{refused}, same: []func() *http.Response{failed}, info: []func() *http.Response{offset}}
	p := script.probes(t, time.Second)
	if _, err := newest(t, p); err == nil || isLogSizeRefusal(err) {
		t.Fatalf("the same-day 500 is returned, not the default read's refusal: %v", err)
	}
}

func TestTheServerOffsetIsRetriedAfterAFailedRequest(t *testing.T) {
	fixClock(t)
	script := &probeScript{
		def:  []func() *http.Response{refused},
		same: []func() *http.Response{process("111")},
		info: []func() *http.Response{failed, offset},
	}
	p := script.probes(t, time.Second)
	if _, err := newest(t, p); err == nil || isLogSizeRefusal(err) {
		t.Fatalf("a failed offset request is returned as itself, got %v", err)
	}
	if id, err := newest(t, p); err != nil || id != "111" {
		t.Fatalf("the offset is asked again on the next refusal: %q %v", id, err)
	}
	if got := script.took(); got != "default,info,default,info,same-day" {
		t.Fatalf("requests: %s", got)
	}
}

func TestANoOffsetAnswerSettlesTheFallback(t *testing.T) {
	fixClock(t)
	script := &probeScript{
		def:  []func() *http.Response{refused},
		same: []func() *http.Response{process("111")},
		info: []func() *http.Response{func() *http.Response { return cmdtest.JSONResponse(http.StatusNotFound, `{}`) }},
	}
	p := script.probes(t, time.Second)
	for i := 0; i < 2; i++ {
		if _, err := newest(t, p); !isLogSizeRefusal(err) {
			t.Fatalf("without an offset the refusal stands: %v", err)
		}
	}
	if got := script.took(); got != "default,info,default" {
		t.Fatalf("a 404 is not asked again: %s", got)
	}
}

// The newest-entry chain (default read, offset, same-day read) runs within
// one request timeout, however slow each request is.
func TestTheProbeChainStaysWithinOneRequestTimeout(t *testing.T) {
	fixClock(t)
	script := &probeScript{
		def:   []func() *http.Response{refused},
		same:  []func() *http.Response{process("111")},
		info:  []func() *http.Response{offset},
		delay: 60 * time.Millisecond,
	}
	p := script.probes(t, 100*time.Millisecond)
	started := time.Now()
	_, err := newest(t, p)
	elapsed := time.Since(started)
	if elapsed > 160*time.Millisecond {
		t.Fatalf("the chain took %s, beyond one 100ms request timeout", elapsed)
	}
	var apiErr *api.APIError
	if err == nil || isLogSizeRefusal(err) || errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest {
		t.Fatalf("a chain cut short reports the timeout, not the refusal: %v", err)
	}
}

func TestSameDayFailuresBelowTheLimitStayTransient(t *testing.T) {
	fixClock(t)
	answers := []func() *http.Response{process("111")}
	for i := 1; i < watchSameDayFailureLimit; i++ {
		answers = append(answers, failed)
	}
	answers = append(answers, process("111"), failed)
	script := &probeScript{def: []func() *http.Response{refused}, same: answers, info: []func() *http.Response{offset}}
	p := script.probes(t, time.Second)
	_, _ = newest(t, p)
	for i := 1; i < watchSameDayFailureLimit; i++ {
		if _, err := newest(t, p); err == nil || isLogSizeRefusal(err) {
			t.Fatalf("failure %d of %d is transient, got %v", i, watchSameDayFailureLimit-1, err)
		}
	}
	if id, err := newest(t, p); err != nil || id != "111" {
		t.Fatalf("a success after %d failures reads the process: %q %v", watchSameDayFailureLimit-1, id, err)
	}
	if _, err := newest(t, p); err == nil || isLogSizeRefusal(err) {
		t.Fatalf("the success resets the count, so the next failure is transient again: %v", err)
	}
}

func TestSameDayFailuresAtTheLimitReportTheSizeRefusal(t *testing.T) {
	fixClock(t)
	script := &probeScript{def: []func() *http.Response{refused}, same: []func() *http.Response{process("111"), failed}, info: []func() *http.Response{offset}}
	p := script.probes(t, time.Second)
	_, _ = newest(t, p)
	script.took()
	for i := 1; i <= watchSameDayFailureLimit; i++ {
		_, err := newest(t, p)
		if refusal := isLogSizeRefusal(err); refusal != (i == watchSameDayFailureLimit) {
			t.Fatalf("failure %d: refusal %v (%v)", i, refusal, err)
		}
	}
	got := strings.Split(script.took(), ",")
	if strings.Join(got[len(got)-2:], ",") != "same-day,default" {
		t.Fatalf("the failure at the limit falls back to one default read, no second same-day read: %v", got)
	}
	// Fresh path: refused, then the same-day read keeps failing.
	script = &probeScript{def: []func() *http.Response{refused}, same: []func() *http.Response{failed}, info: []func() *http.Response{offset}}
	p = script.probes(t, time.Second)
	for i := 1; i <= watchSameDayFailureLimit; i++ {
		_, err := newest(t, p)
		if refusal := isLogSizeRefusal(err); refusal != (i == watchSameDayFailureLimit) {
			t.Fatalf("fresh path failure %d: refusal %v (%v)", i, refusal, err)
		}
	}
}

// The process changes at tick 4 after a real outage, but every same-day
// read after the baseline fails: once the failures reach the limit the
// probe reports the size refusal, and the restart cycle lands the watch.
func TestPersistentSameDayFailuresLetTheRestartCycleLand(t *testing.T) {
	fixClock(t)
	env := &fakeDeployEnv{downTicks: map[int]bool{2: true, 3: true}, landTick: 4, yesterdayOversized: true, serverOffset: "00:00:00", sameDayFailsFrom: 1}
	lines, err := runWatch(t, env, "--restart-min-down", "0")
	if err != nil {
		t.Fatalf("watch failed: %v\n%v", err, phasesOf(lines))
	}
	if landed := detailOf(phaseLine(lines, "landed")); landed["basis"] != "restart-cycle" {
		t.Fatalf("expected a restart-cycle landing: %v", phasesOf(lines))
	}
}

// At baseline, a failure on the same-day path after a size refusal arms
// the watch on the restart-cycle basis, as before the same-day read, rather
// than refusing to arm.
func TestABaselineSameDayFailureArmsOnTheRestartCycle(t *testing.T) {
	for name, env := range map[string]*fakeDeployEnv{
		"server information 500": {downTicks: map[int]bool{}, landTick: 1 << 30, yesterdayOversized: true, serverInfoFails: true},
		"same-day read 500":      {downTicks: map[int]bool{}, landTick: 1 << 30, yesterdayOversized: true, serverOffset: "00:00:00", sameDayFails: map[int]bool{0: true}},
	} {
		t.Run(name, func(t *testing.T) {
			fixClock(t)
			lines, err := runWatch(t, env, "--timeout", "100ms")
			if exitCode(err) != 6 {
				t.Fatalf("expected the watch to arm and time out (exit 6), got %v", err)
			}
			if baseline := detailOf(phaseLine(lines, "baseline")); baseline["landingBasis"] != "restart-cycle" {
				t.Fatalf("expected the restart-cycle basis: %v", baseline)
			}
		})
	}
}
