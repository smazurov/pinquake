package events

import "sync"

// maxQueue bounds each subscriber's backlog. When a subscriber falls this far
// behind, the oldest events are dropped so publishers (e.g. the BLE
// notification handler) never block.
const maxQueue = 4096

// Bus is an in-process pub/sub. Each subscriber gets its own goroutine and
// FIFO queue, so a slow subscriber delays only itself.
type Bus struct {
	mu     sync.RWMutex
	subs   map[uint32][]*subscriber
	closed bool
}

func New() *Bus {
	return &Bus{subs: make(map[uint32][]*subscriber)}
}

// Publish queues ev for every subscriber of its type. Never blocks on
// subscribers.
func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs[ev.Type()] {
		s.enqueue(ev)
	}
}

// Subscribe registers handler, which must be a func(T) for an event type T.
// Returns an unsubscribe func that also stops the subscriber's goroutine.
func (b *Bus) Subscribe(handler any) func() {
	switch h := handler.(type) {
	case func(OrientationEvent):
		return subscribe(b, h)
	case func(BLEStatusEvent):
		return subscribe(b, h)
	case func(BLEScanResultEvent):
		return subscribe(b, h)
	case func(ConfigChangedEvent):
		return subscribe(b, h)
	case func(BatteryEvent):
		return subscribe(b, h)
	case func(HeartbeatEvent):
		return subscribe(b, h)
	case func(LogEntry):
		return subscribe(b, h)
	case func(VizTriggerEvent):
		return subscribe(b, h)
	case func(DelayedOrientationEvent):
		return subscribe(b, h)
	case func(FrameStateEvent):
		return subscribe(b, h)
	case func(OverlayVisibilityEvent):
		return subscribe(b, h)
	case func(OBSStatusEvent):
		return subscribe(b, h)
	default:
		return func() {}
	}
}

// Close stops all subscribers. Later Publish and Subscribe calls are no-ops.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, list := range b.subs {
		for _, s := range list {
			s.stop()
		}
	}
	b.subs = nil
}

func subscribe[T Event](b *Bus, fn func(T)) func() {
	var zero T
	key := zero.Type()
	s := &subscriber{
		handle: func(e Event) { fn(e.(T)) },
		wake:   make(chan struct{}, 1),
		done:   make(chan struct{}),
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return func() {}
	}
	b.subs[key] = append(b.subs[key], s)
	b.mu.Unlock()

	go s.run()
	return func() {
		b.mu.Lock()
		list := b.subs[key]
		for i, other := range list {
			if other == s {
				b.subs[key] = append(list[:i:i], list[i+1:]...)
				break
			}
		}
		b.mu.Unlock()
		s.stop()
	}
}

type subscriber struct {
	handle func(Event)
	wake   chan struct{} // cap 1: "queue is non-empty"
	done   chan struct{}
	once   sync.Once

	mu    sync.Mutex
	queue []Event
}

func (s *subscriber) enqueue(ev Event) {
	s.mu.Lock()
	if len(s.queue) >= maxQueue {
		s.queue = append(s.queue[:0], s.queue[1:]...) // drop oldest
	}
	s.queue = append(s.queue, ev)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *subscriber) stop() { s.once.Do(func() { close(s.done) }) }

func (s *subscriber) run() {
	var batch []Event
	for {
		select {
		case <-s.done:
			return
		case <-s.wake:
		}
		s.mu.Lock()
		batch, s.queue = s.queue, batch[:0]
		s.mu.Unlock()
		for _, ev := range batch {
			select {
			case <-s.done:
				return
			default:
			}
			s.handle(ev)
		}
	}
}
