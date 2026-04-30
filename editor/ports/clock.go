package ports

import "time"

// Clock is the time source. In production it's wall-clock time; tests
// inject a FixedClock so project timestamps are deterministic.
type Clock interface {
	Now() time.Time
}
