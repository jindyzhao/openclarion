package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestConfigRepo_SaveFindUpdateAndListAlertSourceProfile(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	var saved domain.AlertSourceProfile

	if err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Config().SaveAlertSourceProfile(ctx, mustNewAlertSourceProfile(t, "Primary Prometheus"))
		return err
	}); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved profile missing generated fields: %+v", saved)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().FindAlertSourceProfileByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindAlertSourceProfileByID: %v", err)
		}
		if got.Name != "Primary Prometheus" || got.SecretRef != "secret/openclarion/prometheus" || !got.Enabled {
			t.Fatalf("got = %+v", got)
		}
	})

	updated := saved
	updated.Name = "Primary Prometheus Disabled"
	updated.AuthMode = domain.AlertSourceAuthModeNone
	updated.SecretRef = ""
	updated.Enabled = false
	updated.Labels = map[string]string{"env": "prod"}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().UpdateAlertSourceProfile(ctx, updated)
		if err != nil {
			t.Fatalf("UpdateAlertSourceProfile: %v", err)
		}
		if got.SecretRef != "" || got.Enabled || got.Labels["env"] != "prod" {
			t.Fatalf("updated profile = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListAlertSourceProfiles(ctx, 10)
		if err != nil {
			t.Fatalf("ListAlertSourceProfiles: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Primary Prometheus Disabled" {
			t.Fatalf("listed = %+v", listed)
		}
	})
}

func TestConfigRepo_AlertSourceProfileUniqueName(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Config().SaveAlertSourceProfile(ctx, mustNewAlertSourceProfile(t, "Primary Prometheus")); err != nil {
			t.Fatalf("initial save: %v", err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Config().SaveAlertSourceProfile(ctx, mustNewAlertSourceProfile(t, "Primary Prometheus"))
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save err = %v, want ErrAlreadyExists", err)
	}
}

func TestConfigRepo_AlertSourceProfileNotFoundAndInvalidInput(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Config().FindAlertSourceProfileByID(ctx, 404)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("find missing err = %v, want ErrNotFound", err)
		}
		_, err = uow.Config().FindAlertSourceProfileByID(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("find zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.Config().ListAlertSourceProfiles(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list zero err = %v, want ErrInvariantViolation", err)
		}
		profile := mustNewAlertSourceProfile(t, "Missing")
		_, err = uow.Config().UpdateAlertSourceProfile(ctx, profile)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("update zero id err = %v, want ErrInvariantViolation", err)
		}
		profile.ID = 404
		_, err = uow.Config().UpdateAlertSourceProfile(ctx, profile)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing err = %v, want ErrNotFound", err)
		}
	})
}

func TestConfigRepo_ListAlertSourceProfilesOrdersByUpdatedAt(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		oldest := mustNewAlertSourceProfile(t, "Oldest")
		oldest.UpdatedAt = base
		if _, err := uow.Config().SaveAlertSourceProfile(ctx, oldest); err != nil {
			t.Fatalf("save oldest: %v", err)
		}
		newest := mustNewAlertSourceProfile(t, "Newest")
		newest.BaseURL = "https://alertmanager.example.test"
		newest.Kind = domain.AlertSourceKindAlertmanager
		newest.UpdatedAt = base.Add(time.Minute)
		if _, err := uow.Config().SaveAlertSourceProfile(ctx, newest); err != nil {
			t.Fatalf("save newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListAlertSourceProfiles(ctx, 1)
		if err != nil {
			t.Fatalf("ListAlertSourceProfiles: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Newest" {
			t.Fatalf("listed = %+v, want Newest first", listed)
		}
	})
}

func TestConfigRepo_SaveFindUpdateAndListGroupingPolicy(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	var saved domain.GroupingPolicy

	if err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Config().SaveGroupingPolicy(ctx, mustNewGroupingPolicy(t, "Default grouping"))
		return err
	}); err != nil {
		t.Fatalf("save policy: %v", err)
	}
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved policy missing generated fields: %+v", saved)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().FindGroupingPolicyByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindGroupingPolicyByID: %v", err)
		}
		if got.Name != "Default grouping" || got.SeverityKey != "severity" || !got.Enabled {
			t.Fatalf("got = %+v", got)
		}
	})

	updated := saved
	updated.Name = "Service grouping"
	updated.DimensionKeys = []string{"service"}
	updated.SourceFilter = []string{"prometheus"}
	updated.Enabled = false
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().UpdateGroupingPolicy(ctx, updated)
		if err != nil {
			t.Fatalf("UpdateGroupingPolicy: %v", err)
		}
		if got.Name != "Service grouping" || got.Enabled || len(got.SourceFilter) != 1 {
			t.Fatalf("updated policy = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListGroupingPolicies(ctx, 10)
		if err != nil {
			t.Fatalf("ListGroupingPolicies: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Service grouping" {
			t.Fatalf("listed = %+v", listed)
		}
	})
}

func TestConfigRepo_GroupingPolicyUniqueName(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Config().SaveGroupingPolicy(ctx, mustNewGroupingPolicy(t, "Default grouping")); err != nil {
			t.Fatalf("initial save: %v", err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Config().SaveGroupingPolicy(ctx, mustNewGroupingPolicy(t, "Default grouping"))
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save err = %v, want ErrAlreadyExists", err)
	}
}

func TestConfigRepo_GroupingPolicyNotFoundAndInvalidInput(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Config().FindGroupingPolicyByID(ctx, 404)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("find missing err = %v, want ErrNotFound", err)
		}
		_, err = uow.Config().FindGroupingPolicyByID(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("find zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.Config().ListGroupingPolicies(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list zero err = %v, want ErrInvariantViolation", err)
		}
		policy := mustNewGroupingPolicy(t, "Missing")
		_, err = uow.Config().UpdateGroupingPolicy(ctx, policy)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("update zero id err = %v, want ErrInvariantViolation", err)
		}
		policy.ID = 404
		_, err = uow.Config().UpdateGroupingPolicy(ctx, policy)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing err = %v, want ErrNotFound", err)
		}
	})
}

func TestConfigRepo_ListGroupingPoliciesOrdersByUpdatedAt(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		oldest := mustNewGroupingPolicy(t, "Oldest")
		oldest.UpdatedAt = base
		if _, err := uow.Config().SaveGroupingPolicy(ctx, oldest); err != nil {
			t.Fatalf("save oldest: %v", err)
		}
		newest := mustNewGroupingPolicy(t, "Newest")
		newest.UpdatedAt = base.Add(time.Minute)
		if _, err := uow.Config().SaveGroupingPolicy(ctx, newest); err != nil {
			t.Fatalf("save newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListGroupingPolicies(ctx, 1)
		if err != nil {
			t.Fatalf("ListGroupingPolicies: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Newest" {
			t.Fatalf("listed = %+v, want Newest first", listed)
		}
	})
}

func TestConfigRepo_SaveFindUpdateAndListReportWorkflowPolicy(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	var saved domain.ReportWorkflowPolicy

	if err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Config().SaveReportWorkflowPolicy(ctx, mustNewReportWorkflowPolicy(t, "Default report workflow"))
		return err
	}); err != nil {
		t.Fatalf("save policy: %v", err)
	}
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved policy missing generated fields: %+v", saved)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().FindReportWorkflowPolicyByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindReportWorkflowPolicyByID: %v", err)
		}
		if got.Name != "Default report workflow" ||
			got.AlertSourceProfileID != 1 ||
			got.GroupingPolicyID != 2 ||
			got.ReportNotificationChannelProfileID != 3 ||
			got.Enabled {
			t.Fatalf("got = %+v", got)
		}
	})

	enabledAt := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	updated := saved
	updated.Name = "Cascade report workflow"
	updated.ReportScenario = domain.ReportWorkflowScenarioCascade
	updated.DiagnosisFollowUp = domain.DiagnosisFollowUpModeSuggestRoom
	updated.ReportNotificationChannelProfileID = 0
	updated.Enabled = true
	updated.EnabledAt = &enabledAt
	updated.DisabledAt = nil
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().UpdateReportWorkflowPolicy(ctx, updated)
		if err != nil {
			t.Fatalf("UpdateReportWorkflowPolicy: %v", err)
		}
		if got.Name != "Cascade report workflow" ||
			got.ReportNotificationChannelProfileID != 0 ||
			!got.Enabled ||
			got.EnabledAt == nil ||
			got.DisabledAt != nil {
			t.Fatalf("updated policy = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListReportWorkflowPolicies(ctx, 10)
		if err != nil {
			t.Fatalf("ListReportWorkflowPolicies: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Cascade report workflow" {
			t.Fatalf("listed = %+v", listed)
		}
	})
}

func TestConfigRepo_ReportWorkflowPolicyUniqueName(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Config().SaveReportWorkflowPolicy(ctx, mustNewReportWorkflowPolicy(t, "Default report workflow")); err != nil {
			t.Fatalf("initial save: %v", err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Config().SaveReportWorkflowPolicy(ctx, mustNewReportWorkflowPolicy(t, "Default report workflow"))
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save err = %v, want ErrAlreadyExists", err)
	}
}

func TestConfigRepo_ReportWorkflowPolicyNotFoundAndInvalidInput(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Config().FindReportWorkflowPolicyByID(ctx, 404)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("find missing err = %v, want ErrNotFound", err)
		}
		_, err = uow.Config().FindReportWorkflowPolicyByID(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("find zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.Config().ListReportWorkflowPolicies(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list zero err = %v, want ErrInvariantViolation", err)
		}
		policy := mustNewReportWorkflowPolicy(t, "Missing")
		_, err = uow.Config().UpdateReportWorkflowPolicy(ctx, policy)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("update zero id err = %v, want ErrInvariantViolation", err)
		}
		policy.ID = 404
		_, err = uow.Config().UpdateReportWorkflowPolicy(ctx, policy)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing err = %v, want ErrNotFound", err)
		}
	})
}

func TestConfigRepo_ListReportWorkflowPoliciesOrdersByUpdatedAt(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		oldest := mustNewReportWorkflowPolicy(t, "Oldest")
		oldest.UpdatedAt = base
		if _, err := uow.Config().SaveReportWorkflowPolicy(ctx, oldest); err != nil {
			t.Fatalf("save oldest: %v", err)
		}
		newest := mustNewReportWorkflowPolicy(t, "Newest")
		newest.AlertSourceProfileID = 3
		newest.GroupingPolicyID = 4
		newest.UpdatedAt = base.Add(time.Minute)
		if _, err := uow.Config().SaveReportWorkflowPolicy(ctx, newest); err != nil {
			t.Fatalf("save newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListReportWorkflowPolicies(ctx, 1)
		if err != nil {
			t.Fatalf("ListReportWorkflowPolicies: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Newest" {
			t.Fatalf("listed = %+v, want Newest first", listed)
		}
	})
}

func TestConfigRepo_SaveFindUpdateAndListReportWorkflowSchedule(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	var saved domain.ReportWorkflowSchedule

	if err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Config().SaveReportWorkflowSchedule(ctx, mustNewReportWorkflowSchedule(t, "Hourly reports"))
		return err
	}); err != nil {
		t.Fatalf("save schedule: %v", err)
	}
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved schedule missing generated fields: %+v", saved)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().FindReportWorkflowScheduleByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindReportWorkflowScheduleByID: %v", err)
		}
		if got.Name != "Hourly reports" ||
			got.ReportWorkflowPolicyID != 7 ||
			got.TemporalScheduleID != "openclarion-report-policy-7-hourly" ||
			got.Interval != time.Hour ||
			got.ReplayWindow != 30*time.Minute ||
			got.ReplayDelay != 2*time.Minute ||
			got.ReplayLimit != 1000 ||
			got.CatchupWindow != 10*time.Minute ||
			got.Enabled {
			t.Fatalf("got = %+v", got)
		}
	})

	enabledAt := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	updated := saved
	updated.Name = "Thirty minute reports"
	updated.TemporalScheduleID = "openclarion-report-policy-7-30m"
	updated.Interval = 30 * time.Minute
	updated.Offset = time.Minute
	updated.ReplayWindow = 15 * time.Minute
	updated.ReplayLimit = 500
	updated.Enabled = true
	updated.EnabledAt = &enabledAt
	updated.DisabledAt = nil
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().UpdateReportWorkflowSchedule(ctx, updated)
		if err != nil {
			t.Fatalf("UpdateReportWorkflowSchedule: %v", err)
		}
		if got.Name != "Thirty minute reports" ||
			got.TemporalScheduleID != "openclarion-report-policy-7-30m" ||
			got.Interval != 30*time.Minute ||
			got.Offset != time.Minute ||
			!got.Enabled ||
			got.EnabledAt == nil ||
			got.DisabledAt != nil {
			t.Fatalf("updated schedule = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListReportWorkflowSchedules(ctx, 10)
		if err != nil {
			t.Fatalf("ListReportWorkflowSchedules: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Thirty minute reports" {
			t.Fatalf("listed = %+v", listed)
		}
	})
}

func TestConfigRepo_ReportWorkflowScheduleUniqueNameAndTemporalID(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Config().SaveReportWorkflowSchedule(ctx, mustNewReportWorkflowSchedule(t, "Hourly reports")); err != nil {
			t.Fatalf("initial save: %v", err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Config().SaveReportWorkflowSchedule(ctx, mustNewReportWorkflowSchedule(t, "Hourly reports"))
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate name save err = %v, want ErrAlreadyExists", err)
	}

	err = integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		schedule := mustNewReportWorkflowSchedule(t, "Another schedule")
		schedule.TemporalScheduleID = "openclarion-report-policy-7-hourly"
		_, serr := uow.Config().SaveReportWorkflowSchedule(ctx, schedule)
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate temporal id save err = %v, want ErrAlreadyExists", err)
	}
}

func TestConfigRepo_ReportWorkflowScheduleNotFoundAndInvalidInput(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Config().FindReportWorkflowScheduleByID(ctx, 404)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("find missing err = %v, want ErrNotFound", err)
		}
		_, err = uow.Config().FindReportWorkflowScheduleByID(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("find zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.Config().ListReportWorkflowSchedules(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list zero err = %v, want ErrInvariantViolation", err)
		}
		schedule := mustNewReportWorkflowSchedule(t, "Missing")
		_, err = uow.Config().UpdateReportWorkflowSchedule(ctx, schedule)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("update zero id err = %v, want ErrInvariantViolation", err)
		}
		schedule.ID = 404
		_, err = uow.Config().UpdateReportWorkflowSchedule(ctx, schedule)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing err = %v, want ErrNotFound", err)
		}
	})
}

func TestConfigRepo_ListReportWorkflowSchedulesOrdersByUpdatedAt(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		oldest := mustNewReportWorkflowSchedule(t, "Oldest")
		oldest.UpdatedAt = base
		if _, err := uow.Config().SaveReportWorkflowSchedule(ctx, oldest); err != nil {
			t.Fatalf("save oldest: %v", err)
		}
		newest := mustNewReportWorkflowSchedule(t, "Newest")
		newest.TemporalScheduleID = "openclarion-report-policy-7-newest"
		newest.UpdatedAt = base.Add(time.Minute)
		if _, err := uow.Config().SaveReportWorkflowSchedule(ctx, newest); err != nil {
			t.Fatalf("save newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListReportWorkflowSchedules(ctx, 1)
		if err != nil {
			t.Fatalf("ListReportWorkflowSchedules: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Newest" {
			t.Fatalf("listed = %+v, want Newest first", listed)
		}
	})
}

func TestConfigRepo_SaveFindUpdateAndListDiagnosisToolTemplate(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	var saved domain.DiagnosisToolTemplate

	if err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Config().SaveDiagnosisToolTemplate(ctx, mustNewDiagnosisToolTemplate(t, "CPU saturation range"))
		return err
	}); err != nil {
		t.Fatalf("save template: %v", err)
	}
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved template missing generated fields: %+v", saved)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().FindDiagnosisToolTemplateByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindDiagnosisToolTemplateByID: %v", err)
		}
		if got.Name != "CPU saturation range" ||
			got.AlertSourceProfileID != 1 ||
			got.Tool != domain.DiagnosisToolKindMetricRangeQuery ||
			got.QueryTemplate == "" ||
			!got.Enabled {
			t.Fatalf("got = %+v", got)
		}
	})

	updated := saved
	updated.Name = "CPU instant"
	updated.Tool = domain.DiagnosisToolKindMetricQuery
	updated.QueryTemplate = "up"
	updated.DefaultLimit = 3
	updated.DefaultWindow = 0
	updated.MaxWindow = 0
	updated.DefaultStep = 0
	updated.Enabled = false
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().UpdateDiagnosisToolTemplate(ctx, updated)
		if err != nil {
			t.Fatalf("UpdateDiagnosisToolTemplate: %v", err)
		}
		if got.Name != "CPU instant" || got.Tool != domain.DiagnosisToolKindMetricQuery || got.DefaultLimit != 3 || got.Enabled {
			t.Fatalf("updated template = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListDiagnosisToolTemplates(ctx, 10)
		if err != nil {
			t.Fatalf("ListDiagnosisToolTemplates: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "CPU instant" {
			t.Fatalf("listed = %+v", listed)
		}
	})
}

func TestConfigRepo_DiagnosisToolTemplateUniqueName(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Config().SaveDiagnosisToolTemplate(ctx, mustNewDiagnosisToolTemplate(t, "CPU saturation range")); err != nil {
			t.Fatalf("initial save: %v", err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Config().SaveDiagnosisToolTemplate(ctx, mustNewDiagnosisToolTemplate(t, "CPU saturation range"))
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save err = %v, want ErrAlreadyExists", err)
	}
}

func TestConfigRepo_DiagnosisToolTemplateNotFoundAndInvalidInput(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Config().FindDiagnosisToolTemplateByID(ctx, 404)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("find missing err = %v, want ErrNotFound", err)
		}
		_, err = uow.Config().FindDiagnosisToolTemplateByID(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("find zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.Config().ListDiagnosisToolTemplates(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list zero err = %v, want ErrInvariantViolation", err)
		}
		template := mustNewDiagnosisToolTemplate(t, "Missing")
		_, err = uow.Config().UpdateDiagnosisToolTemplate(ctx, template)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("update zero id err = %v, want ErrInvariantViolation", err)
		}
		template.ID = 404
		_, err = uow.Config().UpdateDiagnosisToolTemplate(ctx, template)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing err = %v, want ErrNotFound", err)
		}
	})
}

func TestConfigRepo_ListDiagnosisToolTemplatesOrdersByUpdatedAt(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		oldest := mustNewDiagnosisToolTemplate(t, "Oldest")
		oldest.UpdatedAt = base
		if _, err := uow.Config().SaveDiagnosisToolTemplate(ctx, oldest); err != nil {
			t.Fatalf("save oldest: %v", err)
		}
		newest := mustNewDiagnosisToolTemplate(t, "Newest")
		newest.UpdatedAt = base.Add(time.Minute)
		if _, err := uow.Config().SaveDiagnosisToolTemplate(ctx, newest); err != nil {
			t.Fatalf("save newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListDiagnosisToolTemplates(ctx, 1)
		if err != nil {
			t.Fatalf("ListDiagnosisToolTemplates: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Newest" {
			t.Fatalf("listed = %+v, want Newest first", listed)
		}
	})
}

func TestConfigRepo_SaveFindUpdateAndListNotificationChannelProfile(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	var saved domain.NotificationChannelProfile

	if err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Config().SaveNotificationChannelProfile(ctx, mustNewNotificationChannelProfile(t, "Operations webhook"))
		return err
	}); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if saved.ID == 0 || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved profile missing generated fields: %+v", saved)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().FindNotificationChannelProfileByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindNotificationChannelProfileByID: %v", err)
		}
		if got.Name != "Operations webhook" ||
			got.SecretRef != "secret/openclarion/ops-wecom" ||
			len(got.DeliveryScopes) != 2 ||
			!got.Enabled {
			t.Fatalf("got = %+v", got)
		}
	})

	updated := saved
	updated.Name = "Report webhook"
	updated.DeliveryScopes = []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport}
	updated.Enabled = false
	updated.Labels = map[string]string{"owner": "platform"}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Config().UpdateNotificationChannelProfile(ctx, updated)
		if err != nil {
			t.Fatalf("UpdateNotificationChannelProfile: %v", err)
		}
		if got.Name != "Report webhook" || got.Enabled || len(got.DeliveryScopes) != 1 || got.Labels["owner"] != "platform" {
			t.Fatalf("updated profile = %+v", got)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListNotificationChannelProfiles(ctx, 10)
		if err != nil {
			t.Fatalf("ListNotificationChannelProfiles: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Report webhook" {
			t.Fatalf("listed = %+v", listed)
		}
	})
}

func TestConfigRepo_NotificationChannelProfileUniqueName(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Config().SaveNotificationChannelProfile(ctx, mustNewNotificationChannelProfile(t, "Operations webhook")); err != nil {
			t.Fatalf("initial save: %v", err)
		}
	})

	err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Config().SaveNotificationChannelProfile(ctx, mustNewNotificationChannelProfile(t, "Operations webhook"))
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save err = %v, want ErrAlreadyExists", err)
	}
}

func TestConfigRepo_NotificationChannelProfileNotFoundAndInvalidInput(t *testing.T) {
	resetDB(t)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Config().FindNotificationChannelProfileByID(ctx, 404)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("find missing err = %v, want ErrNotFound", err)
		}
		_, err = uow.Config().FindNotificationChannelProfileByID(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("find zero err = %v, want ErrInvariantViolation", err)
		}
		_, err = uow.Config().ListNotificationChannelProfiles(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("list zero err = %v, want ErrInvariantViolation", err)
		}
		profile := mustNewNotificationChannelProfile(t, "Missing")
		_, err = uow.Config().UpdateNotificationChannelProfile(ctx, profile)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("update zero id err = %v, want ErrInvariantViolation", err)
		}
		profile.ID = 404
		_, err = uow.Config().UpdateNotificationChannelProfile(ctx, profile)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("update missing err = %v, want ErrNotFound", err)
		}
	})
}

func TestConfigRepo_ListNotificationChannelProfilesOrdersByUpdatedAt(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		oldest := mustNewNotificationChannelProfile(t, "Oldest")
		oldest.UpdatedAt = base
		if _, err := uow.Config().SaveNotificationChannelProfile(ctx, oldest); err != nil {
			t.Fatalf("save oldest: %v", err)
		}
		newest := mustNewNotificationChannelProfile(t, "Newest")
		newest.SecretRef = "secret/openclarion/newest-webhook"
		newest.UpdatedAt = base.Add(time.Minute)
		if _, err := uow.Config().SaveNotificationChannelProfile(ctx, newest); err != nil {
			t.Fatalf("save newest: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		listed, err := uow.Config().ListNotificationChannelProfiles(ctx, 1)
		if err != nil {
			t.Fatalf("ListNotificationChannelProfiles: %v", err)
		}
		if len(listed) != 1 || listed[0].Name != "Newest" {
			t.Fatalf("listed = %+v, want Newest first", listed)
		}
	})
}

func TestConfigRepo_SaveAndListLatestNotificationChannelTestProofs(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	var savedProfile domain.NotificationChannelProfile

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		savedProfile, err = uow.Config().SaveNotificationChannelProfile(ctx, mustNewNotificationChannelProfile(t, "Operations WeCom"))
		if err != nil {
			t.Fatalf("SaveNotificationChannelProfile: %v", err)
		}
		for _, proof := range []domain.NotificationChannelTestProof{
			mustNewNotificationChannelTestProof(t, savedProfile.ID, domain.NotificationChannelTestContentAIDiagnosisSample, base, "old-ai"),
			mustNewNotificationChannelTestProof(t, savedProfile.ID, domain.NotificationChannelTestContentDiagnosisCloseSample, base.Add(time.Minute), "close"),
			mustNewNotificationChannelTestProof(t, savedProfile.ID, domain.NotificationChannelTestContentAIDiagnosisSample, base.Add(2*time.Minute), "new-ai"),
		} {
			if _, err := uow.Config().SaveNotificationChannelTestProof(ctx, proof); err != nil {
				t.Fatalf("SaveNotificationChannelTestProof: %v", err)
			}
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		proofs, err := uow.Config().ListLatestNotificationChannelTestProofs(ctx, savedProfile.ID, 4)
		if err != nil {
			t.Fatalf("ListLatestNotificationChannelTestProofs: %v", err)
		}
		if len(proofs) != 2 ||
			proofs[0].ProviderMessageID != "new-ai" ||
			proofs[1].ProviderMessageID != "close" {
			t.Fatalf("proofs = %+v, want newest per content kind", proofs)
		}

		profile, err := uow.Config().FindNotificationChannelProfileByID(ctx, savedProfile.ID)
		if err != nil {
			t.Fatalf("FindNotificationChannelProfileByID: %v", err)
		}
		if len(profile.LatestTestProofs) != 2 {
			t.Fatalf("profile latest proofs = %+v, want 2", profile.LatestTestProofs)
		}
	})
}

func TestConfigRepo_ListLatestNotificationChannelTestProofsDoesNotCapBeforeContentKind(t *testing.T) {
	resetDB(t)
	base := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	var savedProfile domain.NotificationChannelProfile

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		savedProfile, err = uow.Config().SaveNotificationChannelProfile(ctx, mustNewNotificationChannelProfile(t, "Operations WeCom"))
		if err != nil {
			t.Fatalf("SaveNotificationChannelProfile: %v", err)
		}
		closeProof := mustNewNotificationChannelTestProof(t, savedProfile.ID, domain.NotificationChannelTestContentDiagnosisCloseSample, base, "close-current")
		if _, err := uow.Config().SaveNotificationChannelTestProof(ctx, closeProof); err != nil {
			t.Fatalf("Save close proof: %v", err)
		}
		for i := 0; i < 40; i++ {
			proof := mustNewNotificationChannelTestProof(
				t,
				savedProfile.ID,
				domain.NotificationChannelTestContentAIDiagnosisSample,
				base.Add(time.Duration(i+1)*time.Minute),
				fmt.Sprintf("ai-%02d", i),
			)
			if _, err := uow.Config().SaveNotificationChannelTestProof(ctx, proof); err != nil {
				t.Fatalf("Save AI proof %d: %v", i, err)
			}
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		proofs, err := uow.Config().ListLatestNotificationChannelTestProofs(ctx, savedProfile.ID, 4)
		if err != nil {
			t.Fatalf("ListLatestNotificationChannelTestProofs: %v", err)
		}
		gotByKind := map[domain.NotificationChannelTestContentKind]string{}
		for _, proof := range proofs {
			gotByKind[proof.ContentKind] = proof.ProviderMessageID
		}
		if gotByKind[domain.NotificationChannelTestContentAIDiagnosisSample] != "ai-39" ||
			gotByKind[domain.NotificationChannelTestContentDiagnosisCloseSample] != "close-current" {
			t.Fatalf("proofs = %+v, want latest AI and close proofs", proofs)
		}
	})
}

func mustNewAlertSourceProfile(t *testing.T, name string) domain.AlertSourceProfile {
	t.Helper()
	profile, err := domain.NewAlertSourceProfile(
		name,
		domain.AlertSourceKindPrometheus,
		"https://prometheus.example.test",
		domain.AlertSourceAuthModeBearer,
		"secret/openclarion/prometheus",
		true,
		map[string]string{"env": "test"},
	)
	if err != nil {
		t.Fatalf("NewAlertSourceProfile: %v", err)
	}
	return profile
}

func mustNewNotificationChannelTestProof(
	t *testing.T,
	profileID domain.NotificationChannelProfileID,
	contentKind domain.NotificationChannelTestContentKind,
	checkedAt time.Time,
	providerMessageID string,
) domain.NotificationChannelTestProof {
	t.Helper()
	proof, err := domain.NewNotificationChannelTestProof(
		profileID,
		domain.NotificationChannelKindWeCom,
		domain.NotificationChannelTestStatusSuccess,
		domain.NotificationChannelTestReasonOK,
		"Notification channel test delivery succeeded.",
		contentKind,
		strings.Repeat("a", 64),
		checkedAt,
		providerMessageID,
		"accepted",
	)
	if err != nil {
		t.Fatalf("NewNotificationChannelTestProof: %v", err)
	}
	return proof
}

func mustNewGroupingPolicy(t *testing.T, name string) domain.GroupingPolicy {
	t.Helper()
	policy, err := domain.NewGroupingPolicy(
		name,
		[]string{"alertname", "service"},
		"severity",
		[]string{"prometheus"},
		true,
	)
	if err != nil {
		t.Fatalf("NewGroupingPolicy: %v", err)
	}
	return policy
}

func mustNewReportWorkflowPolicy(t *testing.T, name string) domain.ReportWorkflowPolicy {
	t.Helper()
	policy, err := domain.NewReportWorkflowPolicy(
		name,
		domain.AlertSourceProfileID(1),
		domain.GroupingPolicyID(2),
		domain.NotificationChannelProfileID(3),
		domain.ReportWorkflowTriggerModeManualReplay,
		domain.ReportWorkflowScenarioSingleAlert,
		domain.DiagnosisFollowUpModeDisabled,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowPolicy: %v", err)
	}
	return policy
}

func mustNewReportWorkflowSchedule(t *testing.T, name string) domain.ReportWorkflowSchedule {
	t.Helper()
	schedule, err := domain.NewReportWorkflowSchedule(
		name,
		domain.ReportWorkflowPolicyID(7),
		"openclarion-report-policy-7-hourly",
		time.Hour,
		0,
		30*time.Minute,
		2*time.Minute,
		1000,
		10*time.Minute,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowSchedule: %v", err)
	}
	return schedule
}

func mustNewDiagnosisToolTemplate(t *testing.T, name string) domain.DiagnosisToolTemplate {
	t.Helper()
	template, err := domain.NewDiagnosisToolTemplate(
		name,
		domain.AlertSourceProfileID(1),
		domain.DiagnosisToolKindMetricRangeQuery,
		`rate(container_cpu_usage_seconds_total[5m])`,
		5,
		time.Hour,
		2*time.Hour,
		time.Minute,
		true,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDiagnosisToolTemplate: %v", err)
	}
	return template
}

func mustNewNotificationChannelProfile(t *testing.T, name string) domain.NotificationChannelProfile {
	t.Helper()
	profile, err := domain.NewNotificationChannelProfile(
		name,
		domain.NotificationChannelKindWeCom,
		"secret/openclarion/ops-wecom",
		[]domain.NotificationDeliveryScope{
			domain.NotificationDeliveryScopeDiagnosisClose,
			domain.NotificationDeliveryScopeReport,
		},
		true,
		map[string]string{"owner": "sre"},
	)
	if err != nil {
		t.Fatalf("NewNotificationChannelProfile: %v", err)
	}
	return profile
}
