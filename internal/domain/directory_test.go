package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNewDirectoryDepartmentValidatesAndNormalizes(t *testing.T) {
	syncedAt := time.Date(2026, 6, 29, 9, 1, 2, 123456789, time.FixedZone("CST", 8*60*60))
	sourceUpdatedAt := time.Date(2026, 6, 29, 8, 1, 2, 987654321, time.FixedZone("CST", 8*60*60))

	got, err := NewDirectoryDepartment(
		" ops_iam ",
		" dep-1 ",
		" parent-1 ",
		" SRE ",
		" ",
		" ",
		" IT/Platform ",
		2,
		" iam ",
		12,
		&sourceUpdatedAt,
		syncedAt,
	)
	if err != nil {
		t.Fatalf("NewDirectoryDepartment: %v", err)
	}
	if got.Provider != "ops_iam" || got.ExternalID != "dep-1" || got.ParentExternalID != "parent-1" {
		t.Fatalf("unexpected natural key fields: %#v", got)
	}
	if got.DisplayName != "SRE" || got.Path != "SRE" || got.ParentPath != "IT/Platform" {
		t.Fatalf("unexpected display/path defaults: %#v", got)
	}
	if got.Source != "iam" || got.MemberCount != 12 {
		t.Fatalf("unexpected source fields: %#v", got)
	}
	if !got.SyncedAt.Equal(NormalizeUTCMicro(syncedAt)) {
		t.Fatalf("SyncedAt = %s, want normalized %s", got.SyncedAt, NormalizeUTCMicro(syncedAt))
	}
	if got.SourceUpdatedAt == nil || !got.SourceUpdatedAt.Equal(NormalizeUTCMicro(sourceUpdatedAt)) {
		t.Fatalf("SourceUpdatedAt = %v, want normalized %s", got.SourceUpdatedAt, NormalizeUTCMicro(sourceUpdatedAt))
	}
}

func TestNewDirectoryDepartmentRejectsInvalidInputs(t *testing.T) {
	syncedAt := time.Date(2026, 6, 29, 9, 1, 2, 0, time.UTC)
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "blank provider",
			err: func() error {
				_, err := NewDirectoryDepartment(" ", "dep-1", "", "SRE", "", "", "", 1, "iam", 0, nil, syncedAt)
				return err
			}(),
		},
		{
			name: "parent equals external id",
			err: func() error {
				_, err := NewDirectoryDepartment("ops_iam", "dep-1", "dep-1", "SRE", "", "", "", 1, "iam", 0, nil, syncedAt)
				return err
			}(),
		},
		{
			name: "invalid level",
			err: func() error {
				_, err := NewDirectoryDepartment("ops_iam", "dep-1", "", "SRE", "", "", "", maxDirectoryDepartmentLevel+1, "iam", 0, nil, syncedAt)
				return err
			}(),
		},
		{
			name: "negative member count",
			err: func() error {
				_, err := NewDirectoryDepartment("ops_iam", "dep-1", "", "SRE", "", "", "", 1, "iam", -1, nil, syncedAt)
				return err
			}(),
		},
		{
			name: "zero synced at",
			err: func() error {
				_, err := NewDirectoryDepartment("ops_iam", "dep-1", "", "SRE", "", "", "", 1, "iam", 0, nil, time.Time{})
				return err
			}(),
		},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, ErrInvariantViolation) {
			t.Fatalf("%s err = %v, want ErrInvariantViolation", tt.name, tt.err)
		}
	}
}

func TestNewDirectoryUserValidatesAndNormalizes(t *testing.T) {
	syncedAt := time.Date(2026, 6, 29, 9, 1, 2, 123456789, time.FixedZone("CST", 8*60*60))
	sourceUpdatedAt := time.Date(2026, 6, 29, 8, 1, 2, 987654321, time.FixedZone("CST", 8*60*60))
	departmentPathsIn := []string{" IT/Platform ", "IT/Platform/SRE", "IT/Platform"}
	departmentIDsIn := []string{" dep-2 ", "dep-1", "dep-2", " "}

	got, err := NewDirectoryUser(
		" ops_iam ",
		" iam-user-1 ",
		" ",
		" ",
		" ",
		" USER@EXAMPLE.COM ",
		" SRE ",
		" Platform ",
		" Core ",
		" IT/Platform ",
		departmentPathsIn,
		departmentIDsIn,
		true,
		&sourceUpdatedAt,
		syncedAt,
	)
	if err != nil {
		t.Fatalf("NewDirectoryUser: %v", err)
	}
	if got.Provider != "ops_iam" || got.Subject != "iam-user-1" || got.ExternalID != "iam-user-1" {
		t.Fatalf("unexpected identity fields: %#v", got)
	}
	if got.Username != "iam-user-1" || got.DisplayName != "iam-user-1" || got.Email != "user@example.com" {
		t.Fatalf("unexpected display defaults: %#v", got)
	}
	if got.Department != "Platform" || got.Section != "Core" || got.DepartmentPath != "IT/Platform" {
		t.Fatalf("unexpected department fields: %#v", got)
	}
	if want := []string{"IT/Platform", "IT/Platform/SRE"}; !slices.Equal(got.DepartmentPaths, want) {
		t.Fatalf("DepartmentPaths = %#v, want %#v", got.DepartmentPaths, want)
	}
	if want := []string{"dep-1", "dep-2"}; !slices.Equal(got.DepartmentExternalIDs, want) {
		t.Fatalf("DepartmentExternalIDs = %#v, want %#v", got.DepartmentExternalIDs, want)
	}
	if !got.SyncedAt.Equal(NormalizeUTCMicro(syncedAt)) {
		t.Fatalf("SyncedAt = %s, want normalized %s", got.SyncedAt, NormalizeUTCMicro(syncedAt))
	}
	if got.SourceUpdatedAt == nil || !got.SourceUpdatedAt.Equal(NormalizeUTCMicro(sourceUpdatedAt)) {
		t.Fatalf("SourceUpdatedAt = %v, want normalized %s", got.SourceUpdatedAt, NormalizeUTCMicro(sourceUpdatedAt))
	}
	if !slices.Equal(departmentPathsIn, []string{" IT/Platform ", "IT/Platform/SRE", "IT/Platform"}) {
		t.Fatalf("department paths input was mutated: %#v", departmentPathsIn)
	}
	if !slices.Equal(departmentIDsIn, []string{" dep-2 ", "dep-1", "dep-2", " "}) {
		t.Fatalf("department ids input was mutated: %#v", departmentIDsIn)
	}
}

func TestNewDirectoryUserRejectsInvalidInputs(t *testing.T) {
	syncedAt := time.Date(2026, 6, 29, 9, 1, 2, 0, time.UTC)
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "blank provider",
			err: func() error {
				_, err := NewDirectoryUser(" ", "iam-user-1", "", "", "", "", "", "", "", "", nil, nil, true, nil, syncedAt)
				return err
			}(),
		},
		{
			name: "blank subject",
			err: func() error {
				_, err := NewDirectoryUser("ops_iam", " ", "", "", "", "", "", "", "", "", nil, nil, true, nil, syncedAt)
				return err
			}(),
		},
		{
			name: "section too long",
			err: func() error {
				_, err := NewDirectoryUser("ops_iam", "iam-user-1", "", "", "", "", "", "", strings.Repeat("x", maxDirectoryDepartmentLen+1), "", nil, nil, true, nil, syncedAt)
				return err
			}(),
		},
		{
			name: "too many department paths",
			err: func() error {
				paths := make([]string, 0, maxDirectoryDepartmentPaths+1)
				for i := range maxDirectoryDepartmentPaths + 1 {
					paths = append(paths, "department-path-"+strings.Repeat("x", i+1))
				}
				_, err := NewDirectoryUser("ops_iam", "iam-user-1", "", "", "", "", "", "", "", "", paths, nil, true, nil, syncedAt)
				return err
			}(),
		},
		{
			name: "zero synced at",
			err: func() error {
				_, err := NewDirectoryUser("ops_iam", "iam-user-1", "", "", "", "", "", "", "", "", nil, nil, true, nil, time.Time{})
				return err
			}(),
		},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, ErrInvariantViolation) {
			t.Fatalf("%s err = %v, want ErrInvariantViolation", tt.name, tt.err)
		}
	}
}

func TestNewDirectorySyncRunsValidateAndNormalize(t *testing.T) {
	syncedAt := time.Date(2026, 6, 29, 9, 1, 2, 123456789, time.FixedZone("CST", 8*60*60))
	updatedAfter := time.Date(2026, 6, 29, 8, 1, 2, 987654321, time.FixedZone("CST", 8*60*60))

	succeeded, err := NewDirectorySyncSucceededRun(" ops_iam ", 100, &updatedAfter, 1, 2, 3, 4, syncedAt)
	if err != nil {
		t.Fatalf("NewDirectorySyncSucceededRun: %v", err)
	}
	if succeeded.Provider != "ops_iam" || succeeded.Status != DirectorySyncRunStatusSucceeded {
		t.Fatalf("unexpected succeeded run identity/status: %#v", succeeded)
	}
	if succeeded.FailureCode != "" || succeeded.FailureMessage != "" {
		t.Fatalf("succeeded run carried failure details: %#v", succeeded)
	}
	if succeeded.UpdatedAfter == nil || !succeeded.UpdatedAfter.Equal(NormalizeUTCMicro(updatedAfter)) {
		t.Fatalf("UpdatedAfter = %v, want normalized %s", succeeded.UpdatedAfter, NormalizeUTCMicro(updatedAfter))
	}

	failed, err := NewDirectorySyncFailedRun("ops_iam", 100, nil, " upstream_unavailable ", " upstream request failed ", 1, 2, 3, 4, syncedAt)
	if err != nil {
		t.Fatalf("NewDirectorySyncFailedRun: %v", err)
	}
	if failed.Status != DirectorySyncRunStatusFailed || failed.FailureCode != "upstream_unavailable" || failed.FailureMessage != "upstream request failed" {
		t.Fatalf("unexpected failed run details: %#v", failed)
	}
}

func TestNewDirectorySyncRunRejectsInvalidInputs(t *testing.T) {
	syncedAt := time.Date(2026, 6, 29, 9, 1, 2, 0, time.UTC)
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "invalid page size",
			err: func() error {
				_, err := NewDirectorySyncSucceededRun("ops_iam", 0, nil, 0, 0, 0, 0, syncedAt)
				return err
			}(),
		},
		{
			name: "failed run missing code",
			err: func() error {
				_, err := NewDirectorySyncFailedRun("ops_iam", 100, nil, " ", "failed", 0, 0, 0, 0, syncedAt)
				return err
			}(),
		},
		{
			name: "failed run missing message",
			err: func() error {
				_, err := NewDirectorySyncFailedRun("ops_iam", 100, nil, "upstream_unavailable", " ", 0, 0, 0, 0, syncedAt)
				return err
			}(),
		},
		{
			name: "negative counters",
			err: func() error {
				_, err := NewDirectorySyncSucceededRun("ops_iam", 100, nil, -1, 0, 0, 0, syncedAt)
				return err
			}(),
		},
		{
			name: "zero synced at",
			err: func() error {
				_, err := NewDirectorySyncSucceededRun("ops_iam", 100, nil, 0, 0, 0, 0, time.Time{})
				return err
			}(),
		},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, ErrInvariantViolation) {
			t.Fatalf("%s err = %v, want ErrInvariantViolation", tt.name, tt.err)
		}
	}
}
