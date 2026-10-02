package ble

import "context"

// Browse reports nearby devices to b while no device is chosen, until ctx
// is done; see Supervisor.Browse.
func (s *Scanner) Browse(ctx context.Context, b Browser) {
	s.sup.Browse(ctx, b)
}
