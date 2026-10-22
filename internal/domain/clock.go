package domain

import "time"

// Clock abstracts the current time so tests can be deterministic.
// Implementations must return UTC times.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock.
type SystemClock struct{}

// Now returns the current time in UTC.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// FixedClock always returns the same instant. Meant for tests.
type FixedClock struct{ T time.Time }

// Now returns the fixed instant in UTC.
func (c FixedClock) Now() time.Time { return c.T.UTC() }
