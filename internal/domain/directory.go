package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	maxDirectoryProviderLen     = 64
	maxDirectoryExternalIDLen   = 256
	maxDirectorySubjectLen      = 256
	maxDirectoryUsernameLen     = 128
	maxDirectoryDisplayNameLen  = 256
	maxDirectoryEmailLen        = 320
	maxDirectoryTitleLen        = 256
	maxDirectoryDepartmentLen   = 256
	maxDirectoryDepartmentPath  = 1024
	maxDirectoryDepartmentPaths = 32
	maxDirectoryDepartmentIDs   = 32
	maxDirectoryDepartmentLevel = 32
	maxDirectorySourceLen       = 64
	maxDirectorySyncPageSize    = 500
	maxDirectorySyncRunStatus   = 32
	maxDirectorySyncFailureCode = 64
	maxDirectorySyncFailureMsg  = 240
)

// DirectorySyncRunStatus describes whether one local directory sync run
// completed or failed after admission.
type DirectorySyncRunStatus string

const (
	// DirectorySyncRunStatusSucceeded marks a run that completed and wrote all
	// intended local projection pages.
	DirectorySyncRunStatusSucceeded DirectorySyncRunStatus = "succeeded"
	// DirectorySyncRunStatusFailed marks a run that failed after request
	// admission. Failure details are sanitized before persistence.
	DirectorySyncRunStatusFailed DirectorySyncRunStatus = "failed"
)

// DirectoryDepartment is OpenClarion's local projection of one upstream
// directory department. Provider and ExternalID form the stable natural key.
// The row is a projection, not the authority; IAM remains the source of truth.
type DirectoryDepartment struct {
	ID               DirectoryDepartmentID
	Provider         string
	ExternalID       string
	ParentExternalID string
	Name             string
	DisplayName      string
	Path             string
	ParentPath       string
	Level            int
	Source           string
	MemberCount      int
	SourceUpdatedAt  *time.Time
	SyncedAt         time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// DirectoryUser is OpenClarion's local projection of one upstream directory
// user. Subject is the stable IAM subject used for login ownership,
// diagnosis-room attribution, and audit records.
type DirectoryUser struct {
	ID                    DirectoryUserID
	Provider              string
	Subject               string
	ExternalID            string
	Username              string
	DisplayName           string
	Email                 string
	JobTitle              string
	Department            string
	Section               string
	DepartmentPath        string
	DepartmentPaths       []string
	DepartmentExternalIDs []string
	Active                bool
	SourceUpdatedAt       *time.Time
	SyncedAt              time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// DirectorySyncRun records one admitted IAM-to-local directory projection sync.
// It stores only bounded request metadata, aggregate counters, and sanitized
// failure details, never upstream raw payloads or credentials.
type DirectorySyncRun struct {
	ID                  DirectorySyncRunID
	Provider            string
	PageSize            int
	UpdatedAfter        *time.Time
	Status              DirectorySyncRunStatus
	FailureCode         string
	FailureMessage      string
	DepartmentPages     int
	UserPages           int
	DepartmentsUpserted int
	UsersUpserted       int
	SyncedAt            time.Time
	CreatedAt           time.Time
}

// NewDirectoryDepartment constructs a validated local department projection.
func NewDirectoryDepartment(
	provider string,
	externalID string,
	parentExternalID string,
	name string,
	displayName string,
	path string,
	parentPath string,
	level int,
	source string,
	memberCount int,
	sourceUpdatedAt *time.Time,
	syncedAt time.Time,
) (DirectoryDepartment, error) {
	provider = strings.TrimSpace(provider)
	externalID = strings.TrimSpace(externalID)
	parentExternalID = strings.TrimSpace(parentExternalID)
	name = strings.TrimSpace(name)
	displayName = strings.TrimSpace(displayName)
	path = strings.TrimSpace(path)
	parentPath = strings.TrimSpace(parentPath)
	source = strings.TrimSpace(source)
	syncedAt = NormalizeUTCMicro(syncedAt)
	sourceUpdatedAt = normalizeOptionalTime(sourceUpdatedAt)

	if provider == "" {
		return DirectoryDepartment{}, fmt.Errorf("directory department: provider must be non-empty: %w", ErrInvariantViolation)
	}
	if len(provider) > maxDirectoryProviderLen {
		return DirectoryDepartment{}, fmt.Errorf("directory department: provider exceeds %d bytes: %w", maxDirectoryProviderLen, ErrInvariantViolation)
	}
	if externalID == "" {
		return DirectoryDepartment{}, fmt.Errorf("directory department: external_id must be non-empty: %w", ErrInvariantViolation)
	}
	if len(externalID) > maxDirectoryExternalIDLen {
		return DirectoryDepartment{}, fmt.Errorf("directory department: external_id exceeds %d bytes: %w", maxDirectoryExternalIDLen, ErrInvariantViolation)
	}
	if parentExternalID == externalID {
		return DirectoryDepartment{}, fmt.Errorf("directory department: parent_external_id must not equal external_id: %w", ErrInvariantViolation)
	}
	if len(parentExternalID) > maxDirectoryExternalIDLen {
		return DirectoryDepartment{}, fmt.Errorf("directory department: parent_external_id exceeds %d bytes: %w", maxDirectoryExternalIDLen, ErrInvariantViolation)
	}
	if name == "" {
		return DirectoryDepartment{}, fmt.Errorf("directory department: name must be non-empty: %w", ErrInvariantViolation)
	}
	if len(name) > maxDirectoryDepartmentLen {
		return DirectoryDepartment{}, fmt.Errorf("directory department: name exceeds %d bytes: %w", maxDirectoryDepartmentLen, ErrInvariantViolation)
	}
	if displayName == "" {
		displayName = name
	}
	if len(displayName) > maxDirectoryDisplayNameLen {
		return DirectoryDepartment{}, fmt.Errorf("directory department: display_name exceeds %d bytes: %w", maxDirectoryDisplayNameLen, ErrInvariantViolation)
	}
	if path == "" {
		path = displayName
	}
	if len(path) > maxDirectoryDepartmentPath {
		return DirectoryDepartment{}, fmt.Errorf("directory department: path exceeds %d bytes: %w", maxDirectoryDepartmentPath, ErrInvariantViolation)
	}
	if len(parentPath) > maxDirectoryDepartmentPath {
		return DirectoryDepartment{}, fmt.Errorf("directory department: parent_path exceeds %d bytes: %w", maxDirectoryDepartmentPath, ErrInvariantViolation)
	}
	if level < 0 || level > maxDirectoryDepartmentLevel {
		return DirectoryDepartment{}, fmt.Errorf("directory department: level must be between 0 and %d: %w", maxDirectoryDepartmentLevel, ErrInvariantViolation)
	}
	if len(source) > maxDirectorySourceLen {
		return DirectoryDepartment{}, fmt.Errorf("directory department: source exceeds %d bytes: %w", maxDirectorySourceLen, ErrInvariantViolation)
	}
	if memberCount < 0 {
		return DirectoryDepartment{}, fmt.Errorf("directory department: member_count must be non-negative: %w", ErrInvariantViolation)
	}
	if syncedAt.IsZero() {
		return DirectoryDepartment{}, fmt.Errorf("directory department: synced_at must be non-zero: %w", ErrInvariantViolation)
	}
	return DirectoryDepartment{
		Provider:         provider,
		ExternalID:       externalID,
		ParentExternalID: parentExternalID,
		Name:             name,
		DisplayName:      displayName,
		Path:             path,
		ParentPath:       parentPath,
		Level:            level,
		Source:           source,
		MemberCount:      memberCount,
		SourceUpdatedAt:  sourceUpdatedAt,
		SyncedAt:         syncedAt,
	}, nil
}

// NewDirectoryUser constructs a validated local user projection.
func NewDirectoryUser(
	provider string,
	subject string,
	externalID string,
	username string,
	displayName string,
	email string,
	jobTitle string,
	department string,
	section string,
	departmentPath string,
	departmentPaths []string,
	departmentExternalIDs []string,
	active bool,
	sourceUpdatedAt *time.Time,
	syncedAt time.Time,
) (DirectoryUser, error) {
	provider = strings.TrimSpace(provider)
	subject = strings.TrimSpace(subject)
	externalID = strings.TrimSpace(externalID)
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	email = strings.TrimSpace(strings.ToLower(email))
	jobTitle = strings.TrimSpace(jobTitle)
	department = strings.TrimSpace(department)
	section = strings.TrimSpace(section)
	departmentPath = strings.TrimSpace(departmentPath)
	syncedAt = NormalizeUTCMicro(syncedAt)
	sourceUpdatedAt = normalizeOptionalTime(sourceUpdatedAt)

	if provider == "" {
		return DirectoryUser{}, fmt.Errorf("directory user: provider must be non-empty: %w", ErrInvariantViolation)
	}
	if len(provider) > maxDirectoryProviderLen {
		return DirectoryUser{}, fmt.Errorf("directory user: provider exceeds %d bytes: %w", maxDirectoryProviderLen, ErrInvariantViolation)
	}
	if subject == "" {
		return DirectoryUser{}, fmt.Errorf("directory user: subject must be non-empty: %w", ErrInvariantViolation)
	}
	if len(subject) > maxDirectorySubjectLen {
		return DirectoryUser{}, fmt.Errorf("directory user: subject exceeds %d bytes: %w", maxDirectorySubjectLen, ErrInvariantViolation)
	}
	if externalID == "" {
		externalID = subject
	}
	if len(externalID) > maxDirectoryExternalIDLen {
		return DirectoryUser{}, fmt.Errorf("directory user: external_id exceeds %d bytes: %w", maxDirectoryExternalIDLen, ErrInvariantViolation)
	}
	if username == "" {
		username = subject
	}
	if len(username) > maxDirectoryUsernameLen {
		return DirectoryUser{}, fmt.Errorf("directory user: username exceeds %d bytes: %w", maxDirectoryUsernameLen, ErrInvariantViolation)
	}
	if displayName == "" {
		displayName = username
	}
	if len(displayName) > maxDirectoryDisplayNameLen {
		return DirectoryUser{}, fmt.Errorf("directory user: display_name exceeds %d bytes: %w", maxDirectoryDisplayNameLen, ErrInvariantViolation)
	}
	if len(email) > maxDirectoryEmailLen {
		return DirectoryUser{}, fmt.Errorf("directory user: email exceeds %d bytes: %w", maxDirectoryEmailLen, ErrInvariantViolation)
	}
	if len(jobTitle) > maxDirectoryTitleLen {
		return DirectoryUser{}, fmt.Errorf("directory user: job_title exceeds %d bytes: %w", maxDirectoryTitleLen, ErrInvariantViolation)
	}
	if len(department) > maxDirectoryDepartmentLen {
		return DirectoryUser{}, fmt.Errorf("directory user: department exceeds %d bytes: %w", maxDirectoryDepartmentLen, ErrInvariantViolation)
	}
	if len(section) > maxDirectoryDepartmentLen {
		return DirectoryUser{}, fmt.Errorf("directory user: section exceeds %d bytes: %w", maxDirectoryDepartmentLen, ErrInvariantViolation)
	}
	if len(departmentPath) > maxDirectoryDepartmentPath {
		return DirectoryUser{}, fmt.Errorf("directory user: department_path exceeds %d bytes: %w", maxDirectoryDepartmentPath, ErrInvariantViolation)
	}
	departmentPaths, err := normalizeDirectoryPaths(departmentPath, departmentPaths)
	if err != nil {
		return DirectoryUser{}, err
	}
	if departmentPath == "" && len(departmentPaths) > 0 {
		departmentPath = departmentPaths[0]
	}
	departmentExternalIDs, err = normalizeDirectoryExternalIDs(departmentExternalIDs)
	if err != nil {
		return DirectoryUser{}, err
	}
	if syncedAt.IsZero() {
		return DirectoryUser{}, fmt.Errorf("directory user: synced_at must be non-zero: %w", ErrInvariantViolation)
	}
	return DirectoryUser{
		Provider:              provider,
		Subject:               subject,
		ExternalID:            externalID,
		Username:              username,
		DisplayName:           displayName,
		Email:                 email,
		JobTitle:              jobTitle,
		Department:            department,
		Section:               section,
		DepartmentPath:        departmentPath,
		DepartmentPaths:       departmentPaths,
		DepartmentExternalIDs: departmentExternalIDs,
		Active:                active,
		SourceUpdatedAt:       sourceUpdatedAt,
		SyncedAt:              syncedAt,
	}, nil
}

// NewDirectorySyncSucceededRun constructs a validated successful sync run
// record.
func NewDirectorySyncSucceededRun(
	provider string,
	pageSize int,
	updatedAfter *time.Time,
	departmentPages int,
	userPages int,
	departmentsUpserted int,
	usersUpserted int,
	syncedAt time.Time,
) (DirectorySyncRun, error) {
	return newDirectorySyncRun(
		provider,
		pageSize,
		updatedAfter,
		DirectorySyncRunStatusSucceeded,
		"",
		"",
		departmentPages,
		userPages,
		departmentsUpserted,
		usersUpserted,
		syncedAt,
	)
}

// NewDirectorySyncFailedRun constructs a validated failed sync run record. The
// caller must pass a stable, sanitized reason code and operator-facing message.
func NewDirectorySyncFailedRun(
	provider string,
	pageSize int,
	updatedAfter *time.Time,
	failureCode string,
	failureMessage string,
	departmentPages int,
	userPages int,
	departmentsUpserted int,
	usersUpserted int,
	syncedAt time.Time,
) (DirectorySyncRun, error) {
	return newDirectorySyncRun(
		provider,
		pageSize,
		updatedAfter,
		DirectorySyncRunStatusFailed,
		failureCode,
		failureMessage,
		departmentPages,
		userPages,
		departmentsUpserted,
		usersUpserted,
		syncedAt,
	)
}

func newDirectorySyncRun(
	provider string,
	pageSize int,
	updatedAfter *time.Time,
	status DirectorySyncRunStatus,
	failureCode string,
	failureMessage string,
	departmentPages int,
	userPages int,
	departmentsUpserted int,
	usersUpserted int,
	syncedAt time.Time,
) (DirectorySyncRun, error) {
	provider = strings.TrimSpace(provider)
	failureCode = strings.TrimSpace(failureCode)
	failureMessage = strings.TrimSpace(failureMessage)
	updatedAfter = normalizeOptionalTime(updatedAfter)
	syncedAt = NormalizeUTCMicro(syncedAt)

	if provider == "" {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: provider must be non-empty: %w", ErrInvariantViolation)
	}
	if len(provider) > maxDirectoryProviderLen {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: provider exceeds %d bytes: %w", maxDirectoryProviderLen, ErrInvariantViolation)
	}
	if status == "" {
		status = DirectorySyncRunStatusSucceeded
	}
	if err := validateDirectorySyncRunStatus(status); err != nil {
		return DirectorySyncRun{}, err
	}
	if len(status) > maxDirectorySyncRunStatus {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: status exceeds %d bytes: %w", maxDirectorySyncRunStatus, ErrInvariantViolation)
	}
	if pageSize < 1 || pageSize > maxDirectorySyncPageSize {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: page_size must be between 1 and %d: %w", maxDirectorySyncPageSize, ErrInvariantViolation)
	}
	if status == DirectorySyncRunStatusSucceeded && (failureCode != "" || failureMessage != "") {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: successful runs must not carry failure details: %w", ErrInvariantViolation)
	}
	if status == DirectorySyncRunStatusFailed && (failureCode == "" || failureMessage == "") {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: failed runs require failure details: %w", ErrInvariantViolation)
	}
	if len(failureCode) > maxDirectorySyncFailureCode {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: failure_code exceeds %d bytes: %w", maxDirectorySyncFailureCode, ErrInvariantViolation)
	}
	if len(failureMessage) > maxDirectorySyncFailureMsg {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: failure_message exceeds %d bytes: %w", maxDirectorySyncFailureMsg, ErrInvariantViolation)
	}
	if departmentPages < 0 || userPages < 0 || departmentsUpserted < 0 || usersUpserted < 0 {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: counters must be non-negative: %w", ErrInvariantViolation)
	}
	if syncedAt.IsZero() {
		return DirectorySyncRun{}, fmt.Errorf("directory sync run: synced_at must be non-zero: %w", ErrInvariantViolation)
	}
	return DirectorySyncRun{
		Provider:            provider,
		PageSize:            pageSize,
		UpdatedAfter:        updatedAfter,
		Status:              status,
		FailureCode:         failureCode,
		FailureMessage:      failureMessage,
		DepartmentPages:     departmentPages,
		UserPages:           userPages,
		DepartmentsUpserted: departmentsUpserted,
		UsersUpserted:       usersUpserted,
		SyncedAt:            syncedAt,
	}, nil
}

func validateDirectorySyncRunStatus(status DirectorySyncRunStatus) error {
	switch status {
	case DirectorySyncRunStatusSucceeded, DirectorySyncRunStatusFailed:
		return nil
	default:
		return fmt.Errorf("directory sync run: unsupported status %q: %w", status, ErrInvariantViolation)
	}
}

func normalizeOptionalTime(in *time.Time) *time.Time {
	if in == nil || in.IsZero() {
		return nil
	}
	out := NormalizeUTCMicro(*in)
	return &out
}

func normalizeDirectoryPaths(primary string, paths []string) ([]string, error) {
	out := make([]string, 0, 1+len(paths))
	seen := map[string]struct{}{}
	add := func(path string) error {
		path = strings.TrimSpace(path)
		if path == "" {
			return nil
		}
		if len(path) > maxDirectoryDepartmentPath {
			return fmt.Errorf("directory user: department_paths contains value exceeding %d bytes: %w", maxDirectoryDepartmentPath, ErrInvariantViolation)
		}
		if _, ok := seen[path]; ok {
			return nil
		}
		seen[path] = struct{}{}
		out = append(out, path)
		return nil
	}
	if err := add(primary); err != nil {
		return nil, err
	}
	for _, path := range paths {
		if err := add(path); err != nil {
			return nil, err
		}
	}
	if len(out) > maxDirectoryDepartmentPaths {
		return nil, fmt.Errorf("directory user: department_paths exceeds %d entries: %w", maxDirectoryDepartmentPaths, ErrInvariantViolation)
	}
	return out, nil
}

func normalizeDirectoryExternalIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if len(id) > maxDirectoryExternalIDLen {
			return nil, fmt.Errorf("directory user: department_external_ids contains value exceeding %d bytes: %w", maxDirectoryExternalIDLen, ErrInvariantViolation)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) > maxDirectoryDepartmentIDs {
		return nil, fmt.Errorf("directory user: department_external_ids exceeds %d entries: %w", maxDirectoryDepartmentIDs, ErrInvariantViolation)
	}
	slices.Sort(out)
	return out, nil
}
