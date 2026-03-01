package events

import "github.com/kelindar/event"

type Bus struct {
	dispatcher *event.Dispatcher
}

func New() *Bus {
	return &Bus{
		dispatcher: event.NewDispatcher(),
	}
}

func (b *Bus) Publish(ev Event) {
	switch e := ev.(type) {
	case OrientationEvent:
		event.Publish(b.dispatcher, e)
	case BLEStatusEvent:
		event.Publish(b.dispatcher, e)
	case BLEScanResultEvent:
		event.Publish(b.dispatcher, e)
	case ConfigChangedEvent:
		event.Publish(b.dispatcher, e)
	}
}

func (b *Bus) Subscribe(handler any) func() {
	switch h := handler.(type) {
	case func(OrientationEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(BLEStatusEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(BLEScanResultEvent):
		return event.Subscribe(b.dispatcher, h)
	case func(ConfigChangedEvent):
		return event.Subscribe(b.dispatcher, h)
	default:
		return func() {}
	}
}
