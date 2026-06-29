package domain

import (
	"errors"
	"testing"
)

func TestRBACAuthorizeAllowsDepartmentScopedResponder(t *testing.T) {
	assignment, err := NewRBACAssignment(
		RBACSubjectKindDepartment,
		"dep-2",
		RBACRoleResponder,
		RBACScopeKindDiagnosisRoom,
		"room-1",
		true,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment: %v", err)
	}

	allowed, err := RBACAuthorize(
		RBACPrincipal{Subject: "iam-user-1", DepartmentKeys: []string{"dep-1", "dep-2"}},
		RBACRequest{
			Permission: RBACPermissionDiagnosisRoomParticipate,
			ScopeKind:  RBACScopeKindDiagnosisRoom,
			ScopeKey:   "room-1",
		},
		[]RBACAssignment{assignment},
	)
	if err != nil {
		t.Fatalf("RBACAuthorize: %v", err)
	}
	if !allowed {
		t.Fatalf("department responder assignment should allow room participation")
	}
}

func TestRBACAuthorizeDeniesWrongScopeAndDisabledAssignments(t *testing.T) {
	disabled, err := NewRBACAssignment(
		RBACSubjectKindUser,
		"iam-user-1",
		RBACRoleAdmin,
		RBACScopeKindGlobal,
		"",
		false,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment disabled: %v", err)
	}
	roomOnly, err := NewRBACAssignment(
		RBACSubjectKindUser,
		"iam-user-1",
		RBACRoleResponder,
		RBACScopeKindDiagnosisRoom,
		"room-1",
		true,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment roomOnly: %v", err)
	}

	allowed, err := RBACAuthorize(
		RBACPrincipal{Subject: "iam-user-1"},
		RBACRequest{
			Permission: RBACPermissionDiagnosisRoomParticipate,
			ScopeKind:  RBACScopeKindDiagnosisRoom,
			ScopeKey:   "room-2",
		},
		[]RBACAssignment{disabled, roomOnly},
	)
	if err != nil {
		t.Fatalf("RBACAuthorize: %v", err)
	}
	if allowed {
		t.Fatalf("wrong scope or disabled assignment should not allow")
	}
}

func TestRBACAuthorizeIgnoresMalformedAssignment(t *testing.T) {
	allowed, err := RBACAuthorize(
		RBACPrincipal{Subject: "iam-user-1"},
		RBACRequest{
			Permission: RBACPermissionReportWorkflowManage,
			ScopeKind:  RBACScopeKindReportWorkflow,
			ScopeKey:   "policy-1",
		},
		[]RBACAssignment{{
			SubjectKind: RBACSubjectKindUser,
			SubjectKey:  "iam-user-1",
			Role:        RBACRoleAdmin,
			ScopeKind:   RBACScopeKindGlobal,
			ScopeKey:    "not-allowed",
			Enabled:     true,
		}},
	)
	if err != nil {
		t.Fatalf("RBACAuthorize: %v", err)
	}
	if allowed {
		t.Fatalf("malformed global assignment should not allow")
	}
}

func TestRBACAuthorizeAdminAllowsManagePermission(t *testing.T) {
	assignment, err := NewRBACAssignment(
		RBACSubjectKindUser,
		"iam-admin",
		RBACRoleAdmin,
		RBACScopeKindGlobal,
		"",
		true,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment: %v", err)
	}

	allowed, err := RBACAuthorize(
		RBACPrincipal{Subject: "iam-admin"},
		RBACRequest{
			Permission: RBACPermissionReportWorkflowManage,
			ScopeKind:  RBACScopeKindReportWorkflow,
			ScopeKey:   "policy-1",
		},
		[]RBACAssignment{assignment},
	)
	if err != nil {
		t.Fatalf("RBACAuthorize: %v", err)
	}
	if !allowed {
		t.Fatalf("admin should allow manage permission")
	}
}

func TestRBACAuthorizeAllowsReportWorkflowScheduleScope(t *testing.T) {
	assignment, err := NewRBACAssignment(
		RBACSubjectKindUser,
		"iam-operator",
		RBACRoleOperator,
		RBACScopeKindReportWorkflowSchedule,
		"9",
		true,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment: %v", err)
	}

	allowed, err := RBACAuthorize(
		RBACPrincipal{Subject: "iam-operator"},
		RBACRequest{
			Permission: RBACPermissionReportWorkflowRead,
			ScopeKind:  RBACScopeKindReportWorkflowSchedule,
			ScopeKey:   "9",
		},
		[]RBACAssignment{assignment},
	)
	if err != nil {
		t.Fatalf("RBACAuthorize: %v", err)
	}
	if !allowed {
		t.Fatalf("operator schedule-scoped assignment should allow schedule read")
	}
}

func TestRBACAuthorizeAllowsGlobalOperationsReadForViewer(t *testing.T) {
	assignment, err := NewRBACAssignment(
		RBACSubjectKindUser,
		"iam-viewer",
		RBACRoleViewer,
		RBACScopeKindGlobal,
		"",
		true,
	)
	if err != nil {
		t.Fatalf("NewRBACAssignment: %v", err)
	}

	allowed, err := RBACAuthorize(
		RBACPrincipal{Subject: "iam-viewer"},
		RBACRequest{
			Permission: RBACPermissionOperationsRead,
			ScopeKind:  RBACScopeKindGlobal,
		},
		[]RBACAssignment{assignment},
	)
	if err != nil {
		t.Fatalf("RBACAuthorize: %v", err)
	}
	if !allowed {
		t.Fatalf("viewer global assignment should allow operations read")
	}
}

func TestNewRBACAssignmentValidatesScopeShape(t *testing.T) {
	_, err := NewRBACAssignment(RBACSubjectKindUser, "iam-user-1", RBACRoleViewer, RBACScopeKindGlobal, "not-allowed", true)
	if !errors.Is(err, ErrInvariantViolation) {
		t.Fatalf("global with scope key err = %v, want ErrInvariantViolation", err)
	}
	_, err = NewRBACAssignment(RBACSubjectKindUser, "iam-user-1", RBACRoleViewer, RBACScopeKindDiagnosisRoom, "", true)
	if !errors.Is(err, ErrInvariantViolation) {
		t.Fatalf("scoped without key err = %v, want ErrInvariantViolation", err)
	}
}
