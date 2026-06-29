package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	maxRBACSubjectKeyLen = 256
	maxRBACScopeKeyLen   = 256
)

// RBACSubjectKind identifies what kind of local directory subject receives a
// role assignment.
type RBACSubjectKind string

const (
	// RBACSubjectKindUser grants a role directly to one authenticated IAM subject.
	RBACSubjectKindUser RBACSubjectKind = "user"
	// RBACSubjectKindDepartment grants a role to members of one local directory department.
	RBACSubjectKindDepartment RBACSubjectKind = "department"
)

// Valid reports whether k is a supported RBAC subject kind.
func (k RBACSubjectKind) Valid() bool {
	switch k {
	case RBACSubjectKindUser, RBACSubjectKindDepartment:
		return true
	}
	return false
}

// RBACScopeKind identifies the OpenClarion resource family covered by a role
// assignment.
type RBACScopeKind string

const (
	// RBACScopeKindGlobal covers all OpenClarion resources.
	RBACScopeKindGlobal RBACScopeKind = "global"
	// RBACScopeKindDiagnosisRoom covers one diagnosis room.
	RBACScopeKindDiagnosisRoom RBACScopeKind = "diagnosis_room"
	// RBACScopeKindAlertSource covers one alert source profile.
	RBACScopeKindAlertSource RBACScopeKind = "alert_source"
	// RBACScopeKindGroupingPolicy covers one alert grouping policy.
	RBACScopeKindGroupingPolicy RBACScopeKind = "grouping_policy"
	// RBACScopeKindReportWorkflow covers one report workflow policy.
	RBACScopeKindReportWorkflow RBACScopeKind = "report_workflow"
	// RBACScopeKindReportWorkflowSchedule covers one report workflow schedule.
	RBACScopeKindReportWorkflowSchedule RBACScopeKind = "report_workflow_schedule"
	// RBACScopeKindNotificationChannel covers one notification channel profile.
	RBACScopeKindNotificationChannel RBACScopeKind = "notification_channel"
	// RBACScopeKindDiagnosisToolTemplate covers one diagnosis tool template.
	RBACScopeKindDiagnosisToolTemplate RBACScopeKind = "diagnosis_tool_template"
)

// Valid reports whether k is a supported RBAC scope kind.
func (k RBACScopeKind) Valid() bool {
	switch k {
	case RBACScopeKindGlobal,
		RBACScopeKindDiagnosisRoom,
		RBACScopeKindAlertSource,
		RBACScopeKindGroupingPolicy,
		RBACScopeKindReportWorkflow,
		RBACScopeKindReportWorkflowSchedule,
		RBACScopeKindNotificationChannel,
		RBACScopeKindDiagnosisToolTemplate:
		return true
	}
	return false
}

// RBACRole is OpenClarion's local role vocabulary.
type RBACRole string

const (
	// RBACRoleAdmin allows all product permissions.
	RBACRoleAdmin RBACRole = "admin"
	// RBACRoleOperator allows operational read and runbook actions without configuration writes.
	RBACRoleOperator RBACRole = "operator"
	// RBACRoleResponder allows diagnosis-room participation.
	RBACRoleResponder RBACRole = "responder"
	// RBACRoleViewer allows read-only access.
	RBACRoleViewer RBACRole = "viewer"
)

// Valid reports whether r is a supported local role.
func (r RBACRole) Valid() bool {
	switch r {
	case RBACRoleAdmin, RBACRoleOperator, RBACRoleResponder, RBACRoleViewer:
		return true
	}
	return false
}

// RBACPermission is the action-level permission checked by usecases and
// transports.
type RBACPermission string

const (
	// RBACPermissionDirectoryRead allows reading local directory projections.
	RBACPermissionDirectoryRead RBACPermission = "directory.read"
	// RBACPermissionDirectoryManage allows synchronizing local directory projections.
	RBACPermissionDirectoryManage RBACPermission = "directory.manage"
	// RBACPermissionRBACManage allows managing local role assignments and preview checks.
	RBACPermissionRBACManage RBACPermission = "rbac.manage"
	// RBACPermissionOperationsRead allows reading operational alerts, reports, evidence, and dashboard summaries.
	RBACPermissionOperationsRead RBACPermission = "operations.read"
	// RBACPermissionDiagnosisRoomRead allows reading diagnosis rooms.
	RBACPermissionDiagnosisRoomRead RBACPermission = "diagnosis_room.read"
	// RBACPermissionDiagnosisRoomParticipate allows submitting diagnosis-room turns.
	RBACPermissionDiagnosisRoomParticipate RBACPermission = "diagnosis_room.participate"
	// RBACPermissionDiagnosisRoomAdminister allows administrative diagnosis-room actions.
	RBACPermissionDiagnosisRoomAdminister RBACPermission = "diagnosis_room.administer"
	// RBACPermissionAlertSourceRead allows reading alert source configuration.
	RBACPermissionAlertSourceRead RBACPermission = "alert_source.read"
	// RBACPermissionAlertSourceManage allows managing alert source configuration.
	RBACPermissionAlertSourceManage RBACPermission = "alert_source.manage"
	// RBACPermissionGroupingPolicyRead allows reading alert grouping policies.
	RBACPermissionGroupingPolicyRead RBACPermission = "grouping_policy.read"
	// RBACPermissionGroupingPolicyManage allows managing alert grouping policies.
	RBACPermissionGroupingPolicyManage RBACPermission = "grouping_policy.manage"
	// RBACPermissionReportWorkflowRead allows reading report workflow configuration.
	RBACPermissionReportWorkflowRead RBACPermission = "report_workflow.read"
	// RBACPermissionReportWorkflowManage allows managing report workflow configuration.
	RBACPermissionReportWorkflowManage RBACPermission = "report_workflow.manage"
	// RBACPermissionNotificationChannelRead allows reading notification channel configuration.
	RBACPermissionNotificationChannelRead RBACPermission = "notification_channel.read"
	// RBACPermissionNotificationChannelManage allows managing notification channel configuration.
	RBACPermissionNotificationChannelManage RBACPermission = "notification_channel.manage"
	// RBACPermissionNotificationChannelTest allows sending notification channel test messages.
	RBACPermissionNotificationChannelTest RBACPermission = "notification_channel.test"
	// RBACPermissionDiagnosisToolTemplateRead allows reading diagnosis tool templates.
	RBACPermissionDiagnosisToolTemplateRead RBACPermission = "diagnosis_tool_template.read"
	// RBACPermissionDiagnosisToolTemplateManage allows managing diagnosis tool templates.
	RBACPermissionDiagnosisToolTemplateManage RBACPermission = "diagnosis_tool_template.manage"
)

// Valid reports whether p is a supported permission.
func (p RBACPermission) Valid() bool {
	switch p {
	case RBACPermissionDirectoryRead,
		RBACPermissionDirectoryManage,
		RBACPermissionRBACManage,
		RBACPermissionOperationsRead,
		RBACPermissionDiagnosisRoomRead,
		RBACPermissionDiagnosisRoomParticipate,
		RBACPermissionDiagnosisRoomAdminister,
		RBACPermissionAlertSourceRead,
		RBACPermissionAlertSourceManage,
		RBACPermissionGroupingPolicyRead,
		RBACPermissionGroupingPolicyManage,
		RBACPermissionReportWorkflowRead,
		RBACPermissionReportWorkflowManage,
		RBACPermissionNotificationChannelRead,
		RBACPermissionNotificationChannelManage,
		RBACPermissionNotificationChannelTest,
		RBACPermissionDiagnosisToolTemplateRead,
		RBACPermissionDiagnosisToolTemplateManage:
		return true
	}
	return false
}

// RBACAssignment grants one role to one subject over a product scope.
type RBACAssignment struct {
	ID          RBACAssignmentID
	SubjectKind RBACSubjectKind
	SubjectKey  string
	Role        RBACRole
	ScopeKind   RBACScopeKind
	ScopeKey    string
	Enabled     bool
	CreatedBy   string
	UpdatedBy   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RBACPrincipal is the local authorization subject derived from the authenticated
// IAM subject and the local directory projection.
type RBACPrincipal struct {
	Subject        string
	DepartmentKeys []string
}

// RBACRequest describes one authorization decision.
type RBACRequest struct {
	Permission RBACPermission
	ScopeKind  RBACScopeKind
	ScopeKey   string
}

// NewRBACAssignment constructs a validated local role assignment.
func NewRBACAssignment(subjectKind RBACSubjectKind, subjectKey string, role RBACRole, scopeKind RBACScopeKind, scopeKey string, enabled bool) (RBACAssignment, error) {
	subjectKey = strings.TrimSpace(subjectKey)
	scopeKey = strings.TrimSpace(scopeKey)
	if !subjectKind.Valid() {
		return RBACAssignment{}, fmt.Errorf("rbac assignment: subject_kind %q is unsupported: %w", subjectKind, ErrInvariantViolation)
	}
	if subjectKey == "" {
		return RBACAssignment{}, fmt.Errorf("rbac assignment: subject_key must be non-empty: %w", ErrInvariantViolation)
	}
	if len(subjectKey) > maxRBACSubjectKeyLen {
		return RBACAssignment{}, fmt.Errorf("rbac assignment: subject_key exceeds %d bytes: %w", maxRBACSubjectKeyLen, ErrInvariantViolation)
	}
	if !role.Valid() {
		return RBACAssignment{}, fmt.Errorf("rbac assignment: role %q is unsupported: %w", role, ErrInvariantViolation)
	}
	if !scopeKind.Valid() {
		return RBACAssignment{}, fmt.Errorf("rbac assignment: scope_kind %q is unsupported: %w", scopeKind, ErrInvariantViolation)
	}
	if scopeKind == RBACScopeKindGlobal && scopeKey != "" {
		return RBACAssignment{}, fmt.Errorf("rbac assignment: global scope must not include scope_key: %w", ErrInvariantViolation)
	}
	if scopeKind != RBACScopeKindGlobal && scopeKey == "" {
		return RBACAssignment{}, fmt.Errorf("rbac assignment: non-global scope requires scope_key: %w", ErrInvariantViolation)
	}
	if len(scopeKey) > maxRBACScopeKeyLen {
		return RBACAssignment{}, fmt.Errorf("rbac assignment: scope_key exceeds %d bytes: %w", maxRBACScopeKeyLen, ErrInvariantViolation)
	}
	return RBACAssignment{
		SubjectKind: subjectKind,
		SubjectKey:  subjectKey,
		Role:        role,
		ScopeKind:   scopeKind,
		ScopeKey:    scopeKey,
		Enabled:     enabled,
	}, nil
}

// RBACAuthorize evaluates local role assignments for one principal and request.
func RBACAuthorize(principal RBACPrincipal, request RBACRequest, assignments []RBACAssignment) (bool, error) {
	principal.Subject = strings.TrimSpace(principal.Subject)
	if principal.Subject == "" {
		return false, fmt.Errorf("rbac authorize: principal subject must be non-empty: %w", ErrInvariantViolation)
	}
	if !request.Permission.Valid() {
		return false, fmt.Errorf("rbac authorize: permission %q is unsupported: %w", request.Permission, ErrInvariantViolation)
	}
	if !request.ScopeKind.Valid() {
		return false, fmt.Errorf("rbac authorize: scope_kind %q is unsupported: %w", request.ScopeKind, ErrInvariantViolation)
	}
	request.ScopeKey = strings.TrimSpace(request.ScopeKey)
	if request.ScopeKind == RBACScopeKindGlobal && request.ScopeKey != "" {
		return false, fmt.Errorf("rbac authorize: global request must not include scope_key: %w", ErrInvariantViolation)
	}
	if request.ScopeKind != RBACScopeKindGlobal && request.ScopeKey == "" {
		return false, fmt.Errorf("rbac authorize: non-global request requires scope_key: %w", ErrInvariantViolation)
	}
	departmentKeys := normalizeRBACDepartmentKeys(principal.DepartmentKeys)
	for _, assignment := range assignments {
		assignment.SubjectKey = strings.TrimSpace(assignment.SubjectKey)
		assignment.ScopeKey = strings.TrimSpace(assignment.ScopeKey)
		if !rbacAssignmentUsable(assignment) {
			continue
		}
		if !rbacSubjectMatches(principal.Subject, departmentKeys, assignment) {
			continue
		}
		if !rbacScopeMatches(request, assignment) {
			continue
		}
		if rbacRoleAllows(assignment.Role, request.Permission) {
			return true, nil
		}
	}
	return false, nil
}

func rbacAssignmentUsable(assignment RBACAssignment) bool {
	if !assignment.Enabled ||
		!assignment.SubjectKind.Valid() ||
		assignment.SubjectKey == "" ||
		len(assignment.SubjectKey) > maxRBACSubjectKeyLen ||
		!assignment.Role.Valid() ||
		!assignment.ScopeKind.Valid() ||
		len(assignment.ScopeKey) > maxRBACScopeKeyLen {
		return false
	}
	if assignment.ScopeKind == RBACScopeKindGlobal {
		return assignment.ScopeKey == ""
	}
	return assignment.ScopeKey != ""
}

func rbacSubjectMatches(subject string, departmentKeys []string, assignment RBACAssignment) bool {
	switch assignment.SubjectKind {
	case RBACSubjectKindUser:
		return assignment.SubjectKey == subject
	case RBACSubjectKindDepartment:
		return slices.Contains(departmentKeys, assignment.SubjectKey)
	default:
		return false
	}
}

func rbacScopeMatches(request RBACRequest, assignment RBACAssignment) bool {
	if assignment.ScopeKind == RBACScopeKindGlobal {
		return true
	}
	return assignment.ScopeKind == request.ScopeKind && assignment.ScopeKey == request.ScopeKey
}

func rbacRoleAllows(role RBACRole, permission RBACPermission) bool {
	if role == RBACRoleAdmin {
		return true
	}
	allowed := map[RBACRole][]RBACPermission{
		RBACRoleOperator: {
			RBACPermissionDirectoryRead,
			RBACPermissionOperationsRead,
			RBACPermissionDiagnosisRoomRead,
			RBACPermissionDiagnosisRoomParticipate,
			RBACPermissionAlertSourceRead,
			RBACPermissionGroupingPolicyRead,
			RBACPermissionReportWorkflowRead,
			RBACPermissionNotificationChannelRead,
			RBACPermissionNotificationChannelTest,
			RBACPermissionDiagnosisToolTemplateRead,
		},
		RBACRoleResponder: {
			RBACPermissionDirectoryRead,
			RBACPermissionDiagnosisRoomRead,
			RBACPermissionDiagnosisRoomParticipate,
		},
		RBACRoleViewer: {
			RBACPermissionDirectoryRead,
			RBACPermissionOperationsRead,
			RBACPermissionDiagnosisRoomRead,
			RBACPermissionAlertSourceRead,
			RBACPermissionGroupingPolicyRead,
			RBACPermissionReportWorkflowRead,
			RBACPermissionNotificationChannelRead,
			RBACPermissionDiagnosisToolTemplateRead,
		},
	}
	return slices.Contains(allowed[role], permission)
}

func normalizeRBACDepartmentKeys(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, key := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}
