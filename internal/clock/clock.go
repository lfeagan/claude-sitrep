// Package clock lets code and tests agree on what time it is.
package clock

import "time"

// Clock tells the time.
type Clock interface {
	Now() time.Time
}

// Real is the wall clock.
type Real struct{}

func (Real) Now() time.Time { return time.Now() }

// Fixed always answers the same time, for tests.
type Fixed time.Time

func (f Fixed) Now() time.Time { return time.Time(f) }
