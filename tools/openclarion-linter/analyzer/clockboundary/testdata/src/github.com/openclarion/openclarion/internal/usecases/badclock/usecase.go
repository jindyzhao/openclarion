package badclock

import "time"

func run() time.Time {
	return time.Now().UTC() // want "core domain/usecase code must not call time.Now directly"
}
