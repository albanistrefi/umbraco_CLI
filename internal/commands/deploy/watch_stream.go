package deploy

import (
	"io"
	"sync"
)

// watchStream writes deploy watch's lines to one output stream in order,
// on its own goroutine. Writing only queues, so the watch never does I/O
// under its lock: a reader that stops draining stderr (or stdout) cannot
// hold back the probe loop, the log follower or the other stream.
type watchStream struct {
	out    io.Writer
	mu     sync.Mutex
	ready  *sync.Cond
	queue  [][]byte
	closed bool
	done   chan struct{}
}

func newWatchStream(out io.Writer) *watchStream {
	stream := &watchStream{out: out, done: make(chan struct{})}
	stream.ready = sync.NewCond(&stream.mu)
	go stream.run()
	return stream
}

// Write queues p as written and returns at once.
func (s *watchStream) Write(p []byte) (int, error) {
	line := append([]byte(nil), p...)
	s.mu.Lock()
	s.queue = append(s.queue, line)
	s.mu.Unlock()
	s.ready.Signal()
	return len(p), nil
}

func (s *watchStream) run() {
	defer close(s.done)
	for {
		s.mu.Lock()
		for len(s.queue) == 0 && !s.closed {
			s.ready.Wait()
		}
		batch := s.queue
		s.queue = nil
		closed := s.closed
		s.mu.Unlock()
		for _, line := range batch {
			_, _ = s.out.Write(line)
		}
		if closed && len(batch) == 0 {
			return
		}
	}
}

// Close writes out everything queued, then stops the goroutine.
func (s *watchStream) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.ready.Signal()
	<-s.done
}
