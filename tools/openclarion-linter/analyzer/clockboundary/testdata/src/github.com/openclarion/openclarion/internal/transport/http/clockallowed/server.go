package clockallowed

import "time"

func nowAtBoundary() time.Time {
	return time.Now().UTC()
}
