package domain

import (
	"testing"
	"time"
)

func TestNormalizeUTCMicro(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "zero time is returned unchanged",
			in:   time.Time{},
			want: time.Time{},
		},
		{
			name: "non-utc is normalised to utc",
			in:   time.Date(2026, 5, 22, 18, 30, 0, 0, loc),
			want: time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC),
		},
		{
			name: "nanoseconds are truncated to microseconds",
			in:   time.Date(2026, 5, 22, 10, 30, 0, 123456789, time.UTC),
			want: time.Date(2026, 5, 22, 10, 30, 0, 123456000, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeUTCMicro(tt.in)
			if !got.Equal(tt.want) {
				t.Fatalf("NormalizeUTCMicro(%s) = %s, want %s", tt.in, got, tt.want)
			}
			if !tt.in.IsZero() && got.Location() != time.UTC {
				t.Fatalf("NormalizeUTCMicro must return UTC, got %s", got.Location())
			}
		})
	}
}
