package events

// SubscribeToChannel forwards events of type T to ch, dropping them when ch
// is full so a slow SSE client never backs up the bus.
func SubscribeToChannel[T Event](bus *Bus, ch chan<- any) func() {
	return subscribe(bus, func(e T) {
		select {
		case ch <- e:
		default:
		}
	})
}
