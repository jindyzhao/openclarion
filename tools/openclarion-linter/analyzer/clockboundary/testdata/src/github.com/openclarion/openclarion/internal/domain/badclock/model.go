package badclock

import "time"

func mark() time.Time {
	return time.Now() // want "core domain/usecase code must not call time.Now directly"
}
