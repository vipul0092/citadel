package control

// replayPkt carries the replay batch delivered to a new subscriber.
type replayPkt struct {
	gap    *gapInfo
	events []Event
}

// sub is the per-attacher subscription state.
//
// Two-stage design:
//   - replayCh (buffered 1): written once by Subscribe with the historic snapshot;
//     the attacher goroutine drains it before entering the live loop.
//   - liveCh (buffered subQueueSize): receives live events from Emit; if full at Emit
//     time the attacher is dropped (slow-attacher policy).
//   - gameCh (buffered subQueueSize, nil unless wantGame): receives game payloads from
//     EmitGame; same slow-drop policy.
type sub struct {
	level    Level
	wantGame bool
	replayCh chan replayPkt
	liveCh   chan []byte
	gameCh   chan []byte
	done     chan struct{}
}

func newSub(level Level, wantGame bool) *sub {
	s := &sub{
		level:    level,
		wantGame: wantGame,
		replayCh: make(chan replayPkt, 1),
		liveCh:   make(chan []byte, subQueueSize),
		done:     make(chan struct{}),
	}
	if wantGame {
		s.gameCh = make(chan []byte, subQueueSize)
	}
	return s
}

// trySendLive attempts a non-blocking send to liveCh.
// Returns false when the queue is full (caller should drop this sub).
func (s *sub) trySendLive(data []byte) bool {
	select {
	case s.liveCh <- data:
		return true
	default:
		return false
	}
}

// trySendGame attempts a non-blocking send to gameCh.
// Returns false when the queue is full (caller should drop this sub).
func (s *sub) trySendGame(data []byte) bool {
	if s.gameCh == nil {
		return true // not subscribed; not a failure
	}
	select {
	case s.gameCh <- data:
		return true
	default:
		return false
	}
}

// close signals the attacher goroutine to exit.
func (s *sub) close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}
