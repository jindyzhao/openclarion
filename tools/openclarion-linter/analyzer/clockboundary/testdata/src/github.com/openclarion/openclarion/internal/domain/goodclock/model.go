package goodclock

import "time"

type Clock interface {
	Now() time.Time
}

func mark(now time.Time) time.Time {
	return now.UTC()
}

func markWithClock(clock Clock) time.Time {
	return clock.Now().UTC()
}
