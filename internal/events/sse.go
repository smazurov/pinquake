package events

import "github.com/kelindar/event"

// SubscribeToChannel bridges kelindar/event callback subscriptions to channels.
func SubscribeToChannel[T Event](bus *Bus, ch chan<- any) func() {
	return event.Subscribe(bus.dispatcher, func(e T) {
		select {
		case ch <- e:
		default:
		}
	})
}
