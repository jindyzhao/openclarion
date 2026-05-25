package domain

import "time"

// NormalizeUTCMicro normalises t to UTC and truncates the fractional
// part below a microsecond.
//
// PostgreSQL `timestamptz` accepts microsecond resolution but
// upstream alert sources commonly produce nanosecond-resolution
// timestamps. Comparing those at the application layer (e.g. for the
// AlertEvent natural-unique key (source, canonical_fingerprint,
// starts_at)) requires that ingestion and read paths agree on the
// canonical form. Every domain timestamp marked "UTC,
// microsecond-truncated" in the schema documentation is the result
// of this function.
//
// A zero time.Time is returned unchanged.
func NormalizeUTCMicro(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.UTC().Truncate(time.Microsecond)
}
