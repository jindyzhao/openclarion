package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertsourceprofile"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosistooltemplate"
	"github.com/openclarion/openclarion/internal/persistence/ent/directorydepartment"
	"github.com/openclarion/openclarion/internal/persistence/ent/directorysyncrun"
	"github.com/openclarion/openclarion/internal/persistence/ent/directoryuser"
	"github.com/openclarion/openclarion/internal/persistence/ent/groupingpolicy"
	"github.com/openclarion/openclarion/internal/persistence/ent/notificationchannelprofile"
	"github.com/openclarion/openclarion/internal/persistence/ent/notificationchanneltestproof"
	"github.com/openclarion/openclarion/internal/persistence/ent/predicate"
	"github.com/openclarion/openclarion/internal/persistence/ent/rbacassignment"
	"github.com/openclarion/openclarion/internal/persistence/ent/reportworkflowpolicy"
	"github.com/openclarion/openclarion/internal/persistence/ent/reportworkflowschedule"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// configRepo is the Ent-backed implementation of
// ports.ConfigurationRepository.
type configRepo struct {
	tx     *ent.Tx
	closed *atomic.Int32
}

var _ ports.ConfigurationRepository = (*configRepo)(nil)

// SaveAlertSourceProfile inserts one operator-managed alert source profile.
func (r *configRepo) SaveAlertSourceProfile(ctx context.Context, p domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertSourceProfile{}, err
	}
	builder := r.tx.AlertSourceProfile.Create().
		SetName(p.Name).
		SetKind(string(p.Kind)).
		SetBaseURL(p.BaseURL).
		SetAuthMode(string(p.AuthMode)).
		SetEnabled(p.Enabled).
		SetLabels(p.Labels)
	if p.SecretRef != "" {
		builder = builder.SetSecretRef(p.SecretRef)
	}
	if !p.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(p.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.AlertSourceProfile{}, asAlreadyExists(err)
	}
	return alertSourceProfileToDomain(saved), nil
}

// UpdateAlertSourceProfile persists mutable profile fields.
func (r *configRepo) UpdateAlertSourceProfile(ctx context.Context, p domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertSourceProfile{}, err
	}
	if p.ID == 0 {
		return domain.AlertSourceProfile{}, fmt.Errorf("update alert source profile: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.AlertSourceProfile.UpdateOneID(int(p.ID)).
		SetName(p.Name).
		SetKind(string(p.Kind)).
		SetBaseURL(p.BaseURL).
		SetAuthMode(string(p.AuthMode)).
		SetEnabled(p.Enabled).
		SetLabels(p.Labels)
	if p.SecretRef == "" {
		builder = builder.ClearSecretRef()
	} else {
		builder = builder.SetSecretRef(p.SecretRef)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.AlertSourceProfile{}, asAlreadyExists(asNotFound(err))
	}
	return alertSourceProfileToDomain(saved), nil
}

// FindAlertSourceProfileByID returns one alert source profile.
func (r *configRepo) FindAlertSourceProfileByID(ctx context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertSourceProfile{}, err
	}
	if id == 0 {
		return domain.AlertSourceProfile{}, fmt.Errorf("find alert source profile: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.AlertSourceProfile.Get(ctx, int(id))
	if err != nil {
		return domain.AlertSourceProfile{}, asNotFound(err)
	}
	return alertSourceProfileToDomain(row), nil
}

// ListAlertSourceProfiles returns the most recently updated profiles.
func (r *configRepo) ListAlertSourceProfiles(ctx context.Context, limit int) ([]domain.AlertSourceProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list alert source profiles: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.AlertSourceProfile.Query().
		Order(alertsourceprofile.ByUpdatedAt(entsql.OrderDesc()), alertsourceprofile.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list alert source profiles: %w", err)
	}
	out := make([]domain.AlertSourceProfile, len(rows))
	for i, row := range rows {
		out[i] = alertSourceProfileToDomain(row)
	}
	return out, nil
}

// SaveGroupingPolicy inserts one operator-managed grouping policy.
func (r *configRepo) SaveGroupingPolicy(ctx context.Context, p domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.GroupingPolicy{}, err
	}
	builder := r.tx.GroupingPolicy.Create().
		SetName(p.Name).
		SetDimensionKeys(p.DimensionKeys).
		SetSeverityKey(p.SeverityKey).
		SetSourceFilter(p.SourceFilter).
		SetEnabled(p.Enabled)
	if !p.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(p.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.GroupingPolicy{}, asAlreadyExists(err)
	}
	return groupingPolicyToDomain(saved), nil
}

// UpdateGroupingPolicy persists mutable grouping policy fields.
func (r *configRepo) UpdateGroupingPolicy(ctx context.Context, p domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.GroupingPolicy{}, err
	}
	if p.ID == 0 {
		return domain.GroupingPolicy{}, fmt.Errorf("update grouping policy: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	saved, err := r.tx.GroupingPolicy.UpdateOneID(int(p.ID)).
		SetName(p.Name).
		SetDimensionKeys(p.DimensionKeys).
		SetSeverityKey(p.SeverityKey).
		SetSourceFilter(p.SourceFilter).
		SetEnabled(p.Enabled).
		Save(ctx)
	if err != nil {
		return domain.GroupingPolicy{}, asAlreadyExists(asNotFound(err))
	}
	return groupingPolicyToDomain(saved), nil
}

// FindGroupingPolicyByID returns one grouping policy.
func (r *configRepo) FindGroupingPolicyByID(ctx context.Context, id domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.GroupingPolicy{}, err
	}
	if id == 0 {
		return domain.GroupingPolicy{}, fmt.Errorf("find grouping policy: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.GroupingPolicy.Get(ctx, int(id))
	if err != nil {
		return domain.GroupingPolicy{}, asNotFound(err)
	}
	return groupingPolicyToDomain(row), nil
}

// ListGroupingPolicies returns the most recently updated grouping policies.
func (r *configRepo) ListGroupingPolicies(ctx context.Context, limit int) ([]domain.GroupingPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list grouping policies: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.GroupingPolicy.Query().
		Order(groupingpolicy.ByUpdatedAt(entsql.OrderDesc()), groupingpolicy.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list grouping policies: %w", err)
	}
	out := make([]domain.GroupingPolicy, len(rows))
	for i, row := range rows {
		out[i] = groupingPolicyToDomain(row)
	}
	return out, nil
}

// SaveReportWorkflowPolicy inserts one operator-managed report workflow policy.
func (r *configRepo) SaveReportWorkflowPolicy(ctx context.Context, p domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ReportWorkflowPolicy{}, err
	}
	builder := r.tx.ReportWorkflowPolicy.Create().
		SetName(p.Name).
		SetAlertSourceProfileID(int(p.AlertSourceProfileID)).
		SetGroupingPolicyID(int(p.GroupingPolicyID)).
		SetTriggerMode(string(p.TriggerMode)).
		SetReportScenario(string(p.ReportScenario)).
		SetDiagnosisFollowUp(string(p.DiagnosisFollowUp)).
		SetEnabled(p.Enabled)
	if p.ReportNotificationChannelProfileID != 0 {
		builder = builder.SetReportNotificationChannelProfileID(int(p.ReportNotificationChannelProfileID))
	}
	if p.EnabledAt != nil {
		builder = builder.SetEnabledAt(*p.EnabledAt)
	}
	if p.DisabledAt != nil {
		builder = builder.SetDisabledAt(*p.DisabledAt)
	}
	if !p.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(p.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.ReportWorkflowPolicy{}, asAlreadyExists(err)
	}
	return reportWorkflowPolicyToDomain(saved), nil
}

// UpdateReportWorkflowPolicy persists mutable report workflow policy fields.
func (r *configRepo) UpdateReportWorkflowPolicy(ctx context.Context, p domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ReportWorkflowPolicy{}, err
	}
	if p.ID == 0 {
		return domain.ReportWorkflowPolicy{}, fmt.Errorf("update report workflow policy: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.ReportWorkflowPolicy.UpdateOneID(int(p.ID)).
		SetName(p.Name).
		SetAlertSourceProfileID(int(p.AlertSourceProfileID)).
		SetGroupingPolicyID(int(p.GroupingPolicyID)).
		SetTriggerMode(string(p.TriggerMode)).
		SetReportScenario(string(p.ReportScenario)).
		SetDiagnosisFollowUp(string(p.DiagnosisFollowUp)).
		SetEnabled(p.Enabled)
	if p.ReportNotificationChannelProfileID == 0 {
		builder = builder.ClearReportNotificationChannelProfileID()
	} else {
		builder = builder.SetReportNotificationChannelProfileID(int(p.ReportNotificationChannelProfileID))
	}
	if p.EnabledAt == nil {
		builder = builder.ClearEnabledAt()
	} else {
		builder = builder.SetEnabledAt(*p.EnabledAt)
	}
	if p.DisabledAt == nil {
		builder = builder.ClearDisabledAt()
	} else {
		builder = builder.SetDisabledAt(*p.DisabledAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.ReportWorkflowPolicy{}, asAlreadyExists(asNotFound(err))
	}
	return reportWorkflowPolicyToDomain(saved), nil
}

// FindReportWorkflowPolicyByID returns one report workflow policy.
func (r *configRepo) FindReportWorkflowPolicyByID(ctx context.Context, id domain.ReportWorkflowPolicyID) (domain.ReportWorkflowPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ReportWorkflowPolicy{}, err
	}
	if id == 0 {
		return domain.ReportWorkflowPolicy{}, fmt.Errorf("find report workflow policy: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.ReportWorkflowPolicy.Get(ctx, int(id))
	if err != nil {
		return domain.ReportWorkflowPolicy{}, asNotFound(err)
	}
	return reportWorkflowPolicyToDomain(row), nil
}

// ListReportWorkflowPolicies returns the most recently updated report workflow
// policies.
func (r *configRepo) ListReportWorkflowPolicies(ctx context.Context, limit int) ([]domain.ReportWorkflowPolicy, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list report workflow policies: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.ReportWorkflowPolicy.Query().
		Order(reportworkflowpolicy.ByUpdatedAt(entsql.OrderDesc()), reportworkflowpolicy.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list report workflow policies: %w", err)
	}
	out := make([]domain.ReportWorkflowPolicy, len(rows))
	for i, row := range rows {
		out[i] = reportWorkflowPolicyToDomain(row)
	}
	return out, nil
}

// SaveReportWorkflowSchedule inserts one operator-managed report workflow
// schedule.
func (r *configRepo) SaveReportWorkflowSchedule(ctx context.Context, s domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ReportWorkflowSchedule{}, err
	}
	builder := r.tx.ReportWorkflowSchedule.Create().
		SetName(s.Name).
		SetReportWorkflowPolicyID(int(s.ReportWorkflowPolicyID)).
		SetTemporalScheduleID(s.TemporalScheduleID).
		SetIntervalNs(int64(s.Interval)).
		SetOffsetNs(int64(s.Offset)).
		SetReplayWindowNs(int64(s.ReplayWindow)).
		SetReplayDelayNs(int64(s.ReplayDelay)).
		SetReplayLimit(s.ReplayLimit).
		SetCatchupWindowNs(int64(s.CatchupWindow)).
		SetEnabled(s.Enabled)
	if s.EnabledAt != nil {
		builder = builder.SetEnabledAt(*s.EnabledAt)
	}
	if s.DisabledAt != nil {
		builder = builder.SetDisabledAt(*s.DisabledAt)
	}
	if !s.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(s.CreatedAt)
	}
	if !s.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(s.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.ReportWorkflowSchedule{}, asAlreadyExists(err)
	}
	return reportWorkflowScheduleToDomain(saved), nil
}

// UpdateReportWorkflowSchedule persists mutable report workflow schedule
// fields.
func (r *configRepo) UpdateReportWorkflowSchedule(ctx context.Context, s domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ReportWorkflowSchedule{}, err
	}
	if s.ID == 0 {
		return domain.ReportWorkflowSchedule{}, fmt.Errorf("update report workflow schedule: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.ReportWorkflowSchedule.UpdateOneID(int(s.ID)).
		SetName(s.Name).
		SetReportWorkflowPolicyID(int(s.ReportWorkflowPolicyID)).
		SetTemporalScheduleID(s.TemporalScheduleID).
		SetIntervalNs(int64(s.Interval)).
		SetOffsetNs(int64(s.Offset)).
		SetReplayWindowNs(int64(s.ReplayWindow)).
		SetReplayDelayNs(int64(s.ReplayDelay)).
		SetReplayLimit(s.ReplayLimit).
		SetCatchupWindowNs(int64(s.CatchupWindow)).
		SetEnabled(s.Enabled)
	if s.EnabledAt == nil {
		builder = builder.ClearEnabledAt()
	} else {
		builder = builder.SetEnabledAt(*s.EnabledAt)
	}
	if s.DisabledAt == nil {
		builder = builder.ClearDisabledAt()
	} else {
		builder = builder.SetDisabledAt(*s.DisabledAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.ReportWorkflowSchedule{}, asAlreadyExists(asNotFound(err))
	}
	return reportWorkflowScheduleToDomain(saved), nil
}

// FindReportWorkflowScheduleByID returns one report workflow schedule.
func (r *configRepo) FindReportWorkflowScheduleByID(ctx context.Context, id domain.ReportWorkflowScheduleID) (domain.ReportWorkflowSchedule, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ReportWorkflowSchedule{}, err
	}
	if id == 0 {
		return domain.ReportWorkflowSchedule{}, fmt.Errorf("find report workflow schedule: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.ReportWorkflowSchedule.Get(ctx, int(id))
	if err != nil {
		return domain.ReportWorkflowSchedule{}, asNotFound(err)
	}
	return reportWorkflowScheduleToDomain(row), nil
}

// ListReportWorkflowSchedules returns the most recently updated report
// workflow schedules.
func (r *configRepo) ListReportWorkflowSchedules(ctx context.Context, limit int) ([]domain.ReportWorkflowSchedule, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list report workflow schedules: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.ReportWorkflowSchedule.Query().
		Order(reportworkflowschedule.ByUpdatedAt(entsql.OrderDesc()), reportworkflowschedule.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list report workflow schedules: %w", err)
	}
	out := make([]domain.ReportWorkflowSchedule, len(rows))
	for i, row := range rows {
		out[i] = reportWorkflowScheduleToDomain(row)
	}
	return out, nil
}

// SaveDiagnosisToolTemplate inserts one operator-managed diagnosis tool
// template.
func (r *configRepo) SaveDiagnosisToolTemplate(ctx context.Context, t domain.DiagnosisToolTemplate) (domain.DiagnosisToolTemplate, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DiagnosisToolTemplate{}, err
	}
	builder := r.tx.DiagnosisToolTemplate.Create().
		SetName(t.Name).
		SetAlertSourceProfileID(int(t.AlertSourceProfileID)).
		SetTool(string(t.Tool)).
		SetDefaultLimit(t.DefaultLimit).
		SetDefaultWindowNs(int64(t.DefaultWindow)).
		SetMaxWindowNs(int64(t.MaxWindow)).
		SetDefaultStepNs(int64(t.DefaultStep)).
		SetEnabled(t.Enabled)
	if t.QueryTemplate != "" {
		builder = builder.SetQueryTemplate(t.QueryTemplate)
	}
	if t.EnabledAt != nil {
		builder = builder.SetEnabledAt(*t.EnabledAt)
	}
	if t.DisabledAt != nil {
		builder = builder.SetDisabledAt(*t.DisabledAt)
	}
	if !t.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(t.CreatedAt)
	}
	if !t.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(t.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.DiagnosisToolTemplate{}, asAlreadyExists(err)
	}
	return diagnosisToolTemplateToDomain(saved), nil
}

// UpdateDiagnosisToolTemplate persists mutable diagnosis tool template fields.
func (r *configRepo) UpdateDiagnosisToolTemplate(ctx context.Context, t domain.DiagnosisToolTemplate) (domain.DiagnosisToolTemplate, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DiagnosisToolTemplate{}, err
	}
	if t.ID == 0 {
		return domain.DiagnosisToolTemplate{}, fmt.Errorf("update diagnosis tool template: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.DiagnosisToolTemplate.UpdateOneID(int(t.ID)).
		SetName(t.Name).
		SetAlertSourceProfileID(int(t.AlertSourceProfileID)).
		SetTool(string(t.Tool)).
		SetDefaultLimit(t.DefaultLimit).
		SetDefaultWindowNs(int64(t.DefaultWindow)).
		SetMaxWindowNs(int64(t.MaxWindow)).
		SetDefaultStepNs(int64(t.DefaultStep)).
		SetEnabled(t.Enabled)
	if t.QueryTemplate == "" {
		builder = builder.ClearQueryTemplate()
	} else {
		builder = builder.SetQueryTemplate(t.QueryTemplate)
	}
	if t.EnabledAt == nil {
		builder = builder.ClearEnabledAt()
	} else {
		builder = builder.SetEnabledAt(*t.EnabledAt)
	}
	if t.DisabledAt == nil {
		builder = builder.ClearDisabledAt()
	} else {
		builder = builder.SetDisabledAt(*t.DisabledAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.DiagnosisToolTemplate{}, asAlreadyExists(asNotFound(err))
	}
	return diagnosisToolTemplateToDomain(saved), nil
}

// FindDiagnosisToolTemplateByID returns one diagnosis tool template.
func (r *configRepo) FindDiagnosisToolTemplateByID(ctx context.Context, id domain.DiagnosisToolTemplateID) (domain.DiagnosisToolTemplate, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DiagnosisToolTemplate{}, err
	}
	if id == 0 {
		return domain.DiagnosisToolTemplate{}, fmt.Errorf("find diagnosis tool template: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.DiagnosisToolTemplate.Get(ctx, int(id))
	if err != nil {
		return domain.DiagnosisToolTemplate{}, asNotFound(err)
	}
	return diagnosisToolTemplateToDomain(row), nil
}

// ListDiagnosisToolTemplates returns the most recently updated diagnosis tool
// templates.
func (r *configRepo) ListDiagnosisToolTemplates(ctx context.Context, limit int) ([]domain.DiagnosisToolTemplate, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list diagnosis tool templates: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.DiagnosisToolTemplate.Query().
		Order(diagnosistooltemplate.ByUpdatedAt(entsql.OrderDesc()), diagnosistooltemplate.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list diagnosis tool templates: %w", err)
	}
	out := make([]domain.DiagnosisToolTemplate, len(rows))
	for i, row := range rows {
		out[i] = diagnosisToolTemplateToDomain(row)
	}
	return out, nil
}

// SaveNotificationChannelProfile inserts one operator-managed notification
// channel profile.
func (r *configRepo) SaveNotificationChannelProfile(ctx context.Context, p domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.NotificationChannelProfile{}, err
	}
	builder := r.tx.NotificationChannelProfile.Create().
		SetName(p.Name).
		SetKind(string(p.Kind)).
		SetSecretRef(p.SecretRef).
		SetDeliveryScopes(notificationDeliveryScopesToStrings(p.DeliveryScopes)).
		SetEnabled(p.Enabled).
		SetLabels(p.Labels)
	if !p.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(p.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.NotificationChannelProfile{}, asAlreadyExists(err)
	}
	return notificationChannelProfileToDomain(saved), nil
}

// UpdateNotificationChannelProfile persists mutable notification channel
// profile fields.
func (r *configRepo) UpdateNotificationChannelProfile(ctx context.Context, p domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.NotificationChannelProfile{}, err
	}
	if p.ID == 0 {
		return domain.NotificationChannelProfile{}, fmt.Errorf("update notification channel profile: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	saved, err := r.tx.NotificationChannelProfile.UpdateOneID(int(p.ID)).
		SetName(p.Name).
		SetKind(string(p.Kind)).
		SetSecretRef(p.SecretRef).
		SetDeliveryScopes(notificationDeliveryScopesToStrings(p.DeliveryScopes)).
		SetEnabled(p.Enabled).
		SetLabels(p.Labels).
		Save(ctx)
	if err != nil {
		return domain.NotificationChannelProfile{}, asAlreadyExists(asNotFound(err))
	}
	return notificationChannelProfileToDomain(saved), nil
}

// FindNotificationChannelProfileByID returns one notification channel profile.
func (r *configRepo) FindNotificationChannelProfileByID(ctx context.Context, id domain.NotificationChannelProfileID) (domain.NotificationChannelProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.NotificationChannelProfile{}, err
	}
	if id == 0 {
		return domain.NotificationChannelProfile{}, fmt.Errorf("find notification channel profile: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.NotificationChannelProfile.Get(ctx, int(id))
	if err != nil {
		return domain.NotificationChannelProfile{}, asNotFound(err)
	}
	profile := notificationChannelProfileToDomain(row)
	proofs, err := r.ListLatestNotificationChannelTestProofs(ctx, profile.ID, 4)
	if err != nil {
		return domain.NotificationChannelProfile{}, err
	}
	profile.LatestTestProofs = proofs
	return profile, nil
}

// ListNotificationChannelProfiles returns the most recently updated
// notification channel profiles.
func (r *configRepo) ListNotificationChannelProfiles(ctx context.Context, limit int) ([]domain.NotificationChannelProfile, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list notification channel profiles: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.NotificationChannelProfile.Query().
		Order(notificationchannelprofile.ByUpdatedAt(entsql.OrderDesc()), notificationchannelprofile.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list notification channel profiles: %w", err)
	}
	out := make([]domain.NotificationChannelProfile, len(rows))
	for i, row := range rows {
		out[i] = notificationChannelProfileToDomain(row)
		proofs, perr := r.ListLatestNotificationChannelTestProofs(ctx, out[i].ID, 4)
		if perr != nil {
			return nil, perr
		}
		out[i].LatestTestProofs = proofs
	}
	return out, nil
}

// SaveNotificationChannelTestProof appends one sanitized channel test proof.
func (r *configRepo) SaveNotificationChannelTestProof(ctx context.Context, proof domain.NotificationChannelTestProof) (domain.NotificationChannelTestProof, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.NotificationChannelTestProof{}, err
	}
	builder := r.tx.NotificationChannelTestProof.Create().
		SetNotificationChannelProfileID(int(proof.NotificationChannelProfileID)).
		SetKind(string(proof.Kind)).
		SetStatus(string(proof.Status)).
		SetReasonCode(string(proof.ReasonCode)).
		SetMessage(proof.Message).
		SetCheckedAt(proof.CheckedAt).
		SetProviderMessageID(proof.ProviderMessageID).
		SetProviderStatus(proof.ProviderStatus)
	if proof.ContentKind != "" {
		builder = builder.SetContentKind(string(proof.ContentKind))
	}
	if proof.ContentSHA256 != "" {
		builder = builder.SetContentSha256(proof.ContentSHA256)
	}
	if !proof.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(proof.CreatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.NotificationChannelTestProof{}, asNotFound(err)
	}
	return notificationChannelTestProofToDomain(saved), nil
}

// ListLatestNotificationChannelTestProofs returns the latest proof per content
// kind for one notification channel profile.
func (r *configRepo) ListLatestNotificationChannelTestProofs(ctx context.Context, profileID domain.NotificationChannelProfileID, limit int) ([]domain.NotificationChannelTestProof, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if profileID <= 0 {
		return nil, fmt.Errorf("list notification channel test proofs: profile id must be positive: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list notification channel test proofs: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	contentKinds := []domain.NotificationChannelTestContentKind{
		"",
		domain.NotificationChannelTestContentTransportSample,
		domain.NotificationChannelTestContentAIDiagnosisSample,
		domain.NotificationChannelTestContentDiagnosisCloseSample,
	}
	out := make([]domain.NotificationChannelTestProof, 0, min(limit, len(contentKinds)))
	for _, contentKind := range contentKinds {
		row, ok, err := r.latestNotificationChannelTestProofForContentKind(ctx, profileID, contentKind)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, notificationChannelTestProofToDomain(row))
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CheckedAt.Equal(out[j].CheckedAt) {
			return out[i].CheckedAt.After(out[j].CheckedAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *configRepo) latestNotificationChannelTestProofForContentKind(
	ctx context.Context,
	profileID domain.NotificationChannelProfileID,
	contentKind domain.NotificationChannelTestContentKind,
) (*ent.NotificationChannelTestProof, bool, error) {
	query := r.tx.NotificationChannelTestProof.Query().
		Where(notificationchanneltestproof.NotificationChannelProfileIDEQ(int(profileID)))
	if contentKind == "" {
		query = query.Where(notificationchanneltestproof.ContentKindIsNil())
	} else {
		query = query.Where(notificationchanneltestproof.ContentKindEQ(string(contentKind)))
	}
	rows, err := query.
		Order(notificationchanneltestproof.ByCheckedAt(entsql.OrderDesc()), notificationchanneltestproof.ByID(entsql.OrderDesc())).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("list notification channel test proofs: %w", err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	return rows[0], true, nil
}

// UpsertDirectoryDepartment inserts or replaces one local directory department
// projection by provider natural key.
func (r *configRepo) UpsertDirectoryDepartment(ctx context.Context, d domain.DirectoryDepartment) (domain.DirectoryDepartment, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectoryDepartment{}, err
	}
	if strings.TrimSpace(d.Provider) == "" || strings.TrimSpace(d.ExternalID) == "" {
		return domain.DirectoryDepartment{}, fmt.Errorf("upsert directory department: provider and external_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.DirectoryDepartment.Query().
		Where(
			directorydepartment.ProviderEQ(d.Provider),
			directorydepartment.ExternalIDEQ(d.ExternalID),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return domain.DirectoryDepartment{}, fmt.Errorf("find directory department for upsert: %w", err)
	}
	if ent.IsNotFound(err) {
		builder := r.tx.DirectoryDepartment.Create().
			SetProvider(d.Provider).
			SetExternalID(d.ExternalID).
			SetName(d.Name).
			SetDisplayName(d.DisplayName).
			SetPath(d.Path).
			SetLevel(d.Level).
			SetMemberCount(d.MemberCount).
			SetSyncedAt(d.SyncedAt)
		if d.ParentExternalID != "" {
			builder = builder.SetParentExternalID(d.ParentExternalID)
		}
		if d.ParentPath != "" {
			builder = builder.SetParentPath(d.ParentPath)
		}
		if d.Source != "" {
			builder = builder.SetSource(d.Source)
		}
		if d.SourceUpdatedAt != nil {
			builder = builder.SetSourceUpdatedAt(*d.SourceUpdatedAt)
		}
		if !d.CreatedAt.IsZero() {
			builder = builder.SetCreatedAt(d.CreatedAt)
		}
		if !d.UpdatedAt.IsZero() {
			builder = builder.SetUpdatedAt(d.UpdatedAt)
		}
		saved, serr := builder.Save(ctx)
		if serr != nil {
			return domain.DirectoryDepartment{}, asAlreadyExists(serr)
		}
		return directoryDepartmentToDomain(saved), nil
	}

	builder := r.tx.DirectoryDepartment.UpdateOneID(row.ID).
		SetName(d.Name).
		SetDisplayName(d.DisplayName).
		SetPath(d.Path).
		SetLevel(d.Level).
		SetMemberCount(d.MemberCount).
		SetSyncedAt(d.SyncedAt)
	if d.ParentExternalID == "" {
		builder = builder.ClearParentExternalID()
	} else {
		builder = builder.SetParentExternalID(d.ParentExternalID)
	}
	if d.ParentPath == "" {
		builder = builder.ClearParentPath()
	} else {
		builder = builder.SetParentPath(d.ParentPath)
	}
	if d.Source == "" {
		builder = builder.ClearSource()
	} else {
		builder = builder.SetSource(d.Source)
	}
	if d.SourceUpdatedAt == nil {
		builder = builder.ClearSourceUpdatedAt()
	} else {
		builder = builder.SetSourceUpdatedAt(*d.SourceUpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.DirectoryDepartment{}, asAlreadyExists(asNotFound(err))
	}
	return directoryDepartmentToDomain(saved), nil
}

// UpsertDirectoryUser inserts or replaces one local directory user projection
// by stable provider subject, converging external-id changes into the same row.
func (r *configRepo) UpsertDirectoryUser(ctx context.Context, u domain.DirectoryUser) (domain.DirectoryUser, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectoryUser{}, err
	}
	if strings.TrimSpace(u.Provider) == "" || strings.TrimSpace(u.Subject) == "" || strings.TrimSpace(u.ExternalID) == "" {
		return domain.DirectoryUser{}, fmt.Errorf("upsert directory user: provider, subject, and external_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	rows, err := r.tx.DirectoryUser.Query().
		Where(directoryuser.Or(
			directoryuser.And(
				directoryuser.ProviderEQ(u.Provider),
				directoryuser.SubjectEQ(u.Subject),
			),
			directoryuser.And(
				directoryuser.ProviderEQ(u.Provider),
				directoryuser.ExternalIDEQ(u.ExternalID),
			),
		)).
		All(ctx)
	if err != nil {
		return domain.DirectoryUser{}, fmt.Errorf("find directory user for upsert: %w", err)
	}
	if len(rows) > 1 {
		return domain.DirectoryUser{}, fmt.Errorf("upsert directory user: provider subject and external_id match different rows: %w", domain.ErrInvariantViolation)
	}
	if len(rows) == 0 {
		builder := r.tx.DirectoryUser.Create().
			SetProvider(u.Provider).
			SetSubject(u.Subject).
			SetExternalID(u.ExternalID).
			SetUsername(u.Username).
			SetDisplayName(u.DisplayName).
			SetDepartmentPaths(u.DepartmentPaths).
			SetDepartmentExternalIds(u.DepartmentExternalIDs).
			SetActive(u.Active).
			SetSyncedAt(u.SyncedAt)
		setDirectoryUserCreateOptionalFields(builder, u)
		if !u.CreatedAt.IsZero() {
			builder = builder.SetCreatedAt(u.CreatedAt)
		}
		if !u.UpdatedAt.IsZero() {
			builder = builder.SetUpdatedAt(u.UpdatedAt)
		}
		saved, serr := builder.Save(ctx)
		if serr != nil {
			return domain.DirectoryUser{}, asAlreadyExists(serr)
		}
		return directoryUserToDomain(saved), nil
	}

	builder := r.tx.DirectoryUser.UpdateOneID(rows[0].ID).
		SetSubject(u.Subject).
		SetExternalID(u.ExternalID).
		SetUsername(u.Username).
		SetDisplayName(u.DisplayName).
		SetDepartmentPaths(u.DepartmentPaths).
		SetDepartmentExternalIds(u.DepartmentExternalIDs).
		SetActive(u.Active).
		SetSyncedAt(u.SyncedAt)
	setDirectoryUserUpdateOptionalFields(builder, u)
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.DirectoryUser{}, asAlreadyExists(asNotFound(err))
	}
	return directoryUserToDomain(saved), nil
}

// FindDirectoryDepartmentByProviderExternalID returns one local department
// projection by provider natural key.
func (r *configRepo) FindDirectoryDepartmentByProviderExternalID(ctx context.Context, provider, externalID string) (domain.DirectoryDepartment, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectoryDepartment{}, err
	}
	provider = strings.TrimSpace(provider)
	externalID = strings.TrimSpace(externalID)
	if provider == "" || externalID == "" {
		return domain.DirectoryDepartment{}, fmt.Errorf("find directory department: provider and external_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.DirectoryDepartment.Query().
		Where(directorydepartment.ProviderEQ(provider), directorydepartment.ExternalIDEQ(externalID)).
		Only(ctx)
	if err != nil {
		return domain.DirectoryDepartment{}, asNotFound(err)
	}
	return directoryDepartmentToDomain(row), nil
}

// FindDirectoryUserByProviderSubject returns one local user projection by
// stable provider subject.
func (r *configRepo) FindDirectoryUserByProviderSubject(ctx context.Context, provider, subject string) (domain.DirectoryUser, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectoryUser{}, err
	}
	provider = strings.TrimSpace(provider)
	subject = strings.TrimSpace(subject)
	if provider == "" || subject == "" {
		return domain.DirectoryUser{}, fmt.Errorf("find directory user: provider and subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.DirectoryUser.Query().
		Where(directoryuser.ProviderEQ(provider), directoryuser.SubjectEQ(subject)).
		Only(ctx)
	if err != nil {
		return domain.DirectoryUser{}, asNotFound(err)
	}
	return directoryUserToDomain(row), nil
}

// ListDirectoryUsersBySubject returns local user projections for one stable
// IAM subject across providers.
func (r *configRepo) ListDirectoryUsersBySubject(ctx context.Context, subject string, limit int) ([]domain.DirectoryUser, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, fmt.Errorf("list directory users by subject: subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list directory users by subject: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.DirectoryUser.Query().
		Where(directoryuser.SubjectEQ(subject)).
		Order(directoryuser.ByProvider(), directoryuser.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list directory users by subject: %w", err)
	}
	out := make([]domain.DirectoryUser, len(rows))
	for i, row := range rows {
		out[i] = directoryUserToDomain(row)
	}
	return out, nil
}

// ListDirectoryDepartments returns local department projections ordered for
// stable operator display.
func (r *configRepo) ListDirectoryDepartments(ctx context.Context, provider string, limit int) ([]domain.DirectoryDepartment, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list directory departments: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	query := r.tx.DirectoryDepartment.Query()
	if provider = strings.TrimSpace(provider); provider != "" {
		query = query.Where(directorydepartment.ProviderEQ(provider))
	}
	rows, err := query.
		Order(directorydepartment.ByPath(), directorydepartment.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list directory departments: %w", err)
	}
	out := make([]domain.DirectoryDepartment, len(rows))
	for i, row := range rows {
		out[i] = directoryDepartmentToDomain(row)
	}
	return out, nil
}

// ListDirectoryUsers returns local user projections ordered for stable operator
// display.
func (r *configRepo) ListDirectoryUsers(ctx context.Context, provider string, limit int) ([]domain.DirectoryUser, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list directory users: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	query := r.tx.DirectoryUser.Query()
	if provider = strings.TrimSpace(provider); provider != "" {
		query = query.Where(directoryuser.ProviderEQ(provider))
	}
	rows, err := query.
		Order(directoryuser.ByDisplayName(), directoryuser.BySubject(), directoryuser.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list directory users: %w", err)
	}
	out := make([]domain.DirectoryUser, len(rows))
	for i, row := range rows {
		out[i] = directoryUserToDomain(row)
	}
	return out, nil
}

// DeactivateStaleDirectoryUsers marks users inactive when a full sync did not
// refresh them at the provided timestamp.
func (r *configRepo) DeactivateStaleDirectoryUsers(ctx context.Context, provider string, syncedAt time.Time) (int, error) {
	if err := checkOpen(r.closed); err != nil {
		return 0, err
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return 0, fmt.Errorf("deactivate stale directory users: provider must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if syncedAt.IsZero() {
		return 0, fmt.Errorf("deactivate stale directory users: synced_at must be non-zero: %w", domain.ErrInvariantViolation)
	}
	updated, err := r.tx.DirectoryUser.Update().
		Where(
			directoryuser.ProviderEQ(provider),
			directoryuser.ActiveEQ(true),
			directoryuser.SyncedAtLT(syncedAt),
		).
		SetActive(false).
		SetSyncedAt(syncedAt).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("deactivate stale directory users: %w", err)
	}
	return updated, nil
}

// SaveDirectorySyncRun appends one admitted local directory projection sync run
// summary.
func (r *configRepo) SaveDirectorySyncRun(ctx context.Context, run domain.DirectorySyncRun) (domain.DirectorySyncRun, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DirectorySyncRun{}, err
	}
	if strings.TrimSpace(run.Provider) == "" {
		return domain.DirectorySyncRun{}, fmt.Errorf("save directory sync run: provider must be non-empty: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.DirectorySyncRun.Create().
		SetProvider(run.Provider).
		SetPageSize(run.PageSize).
		SetStatus(string(run.Status)).
		SetFailureCode(run.FailureCode).
		SetFailureMessage(run.FailureMessage).
		SetDepartmentPages(run.DepartmentPages).
		SetUserPages(run.UserPages).
		SetDepartmentsUpserted(run.DepartmentsUpserted).
		SetUsersUpserted(run.UsersUpserted).
		SetSyncedAt(run.SyncedAt)
	if run.UpdatedAfter != nil {
		builder = builder.SetUpdatedAfter(*run.UpdatedAfter)
	}
	if !run.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(run.CreatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.DirectorySyncRun{}, fmt.Errorf("save directory sync run: %w", err)
	}
	return directorySyncRunToDomain(saved), nil
}

// ListDirectorySyncRuns returns sync runs ordered by newest completion first.
func (r *configRepo) ListDirectorySyncRuns(ctx context.Context, provider string, limit int) ([]domain.DirectorySyncRun, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list directory sync runs: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	query := r.tx.DirectorySyncRun.Query()
	if provider = strings.TrimSpace(provider); provider != "" {
		query = query.Where(directorysyncrun.ProviderEQ(provider))
	}
	rows, err := query.
		Order(directorysyncrun.BySyncedAt(entsql.OrderDesc()), directorysyncrun.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list directory sync runs: %w", err)
	}
	out := make([]domain.DirectorySyncRun, len(rows))
	for i, row := range rows {
		out[i] = directorySyncRunToDomain(row)
	}
	return out, nil
}

func setDirectoryUserCreateOptionalFields(builder *ent.DirectoryUserCreate, u domain.DirectoryUser) {
	if u.Email != "" {
		builder.SetEmail(u.Email)
	}
	if u.JobTitle != "" {
		builder.SetJobTitle(u.JobTitle)
	}
	if u.Department != "" {
		builder.SetDepartment(u.Department)
	}
	if u.Section != "" {
		builder.SetSection(u.Section)
	}
	if u.DepartmentPath != "" {
		builder.SetDepartmentPath(u.DepartmentPath)
	}
	if u.SourceUpdatedAt != nil {
		builder.SetSourceUpdatedAt(*u.SourceUpdatedAt)
	}
}

func setDirectoryUserUpdateOptionalFields(builder *ent.DirectoryUserUpdateOne, u domain.DirectoryUser) {
	if u.Email == "" {
		builder.ClearEmail()
	} else {
		builder.SetEmail(u.Email)
	}
	if u.JobTitle == "" {
		builder.ClearJobTitle()
	} else {
		builder.SetJobTitle(u.JobTitle)
	}
	if u.Department == "" {
		builder.ClearDepartment()
	} else {
		builder.SetDepartment(u.Department)
	}
	if u.Section == "" {
		builder.ClearSection()
	} else {
		builder.SetSection(u.Section)
	}
	if u.DepartmentPath == "" {
		builder.ClearDepartmentPath()
	} else {
		builder.SetDepartmentPath(u.DepartmentPath)
	}
	if u.SourceUpdatedAt == nil {
		builder.ClearSourceUpdatedAt()
	} else {
		builder.SetSourceUpdatedAt(*u.SourceUpdatedAt)
	}
}

// UpsertRBACAssignment inserts or replaces one local role assignment by its
// natural key.
func (r *configRepo) UpsertRBACAssignment(ctx context.Context, a domain.RBACAssignment) (domain.RBACAssignment, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.RBACAssignment{}, err
	}
	if strings.TrimSpace(a.SubjectKey) == "" {
		return domain.RBACAssignment{}, fmt.Errorf("upsert rbac assignment: subject_key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	updatedBy := strings.TrimSpace(a.UpdatedBy)
	if updatedBy == "" {
		return domain.RBACAssignment{}, fmt.Errorf("upsert rbac assignment: updated_by must be non-empty: %w", domain.ErrInvariantViolation)
	}
	createdBy := strings.TrimSpace(a.CreatedBy)
	if createdBy == "" {
		createdBy = updatedBy
	}
	row, err := r.tx.RBACAssignment.Query().
		Where(
			rbacassignment.SubjectKindEQ(string(a.SubjectKind)),
			rbacassignment.SubjectKeyEQ(a.SubjectKey),
			rbacassignment.RoleEQ(string(a.Role)),
			rbacassignment.ScopeKindEQ(string(a.ScopeKind)),
			rbacassignment.ScopeKeyEQ(a.ScopeKey),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return domain.RBACAssignment{}, fmt.Errorf("find rbac assignment for upsert: %w", err)
	}
	if ent.IsNotFound(err) {
		builder := r.tx.RBACAssignment.Create().
			SetSubjectKind(string(a.SubjectKind)).
			SetSubjectKey(a.SubjectKey).
			SetRole(string(a.Role)).
			SetScopeKind(string(a.ScopeKind)).
			SetScopeKey(a.ScopeKey).
			SetEnabled(a.Enabled).
			SetCreatedBy(createdBy).
			SetUpdatedBy(updatedBy)
		if !a.CreatedAt.IsZero() {
			builder = builder.SetCreatedAt(a.CreatedAt)
		}
		if !a.UpdatedAt.IsZero() {
			builder = builder.SetUpdatedAt(a.UpdatedAt)
		}
		saved, serr := builder.Save(ctx)
		if serr != nil {
			return domain.RBACAssignment{}, asAlreadyExists(serr)
		}
		return rbacAssignmentToDomain(saved), nil
	}
	saved, err := r.tx.RBACAssignment.UpdateOneID(row.ID).
		SetEnabled(a.Enabled).
		SetUpdatedBy(updatedBy).
		Save(ctx)
	if err != nil {
		return domain.RBACAssignment{}, asAlreadyExists(asNotFound(err))
	}
	return rbacAssignmentToDomain(saved), nil
}

// ListRBACAssignments returns local role assignments ordered for operator review.
func (r *configRepo) ListRBACAssignments(ctx context.Context, limit int) ([]domain.RBACAssignment, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list rbac assignments: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.RBACAssignment.Query().
		Order(rbacassignment.ByUpdatedAt(entsql.OrderDesc()), rbacassignment.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rbac assignments: %w", err)
	}
	out := make([]domain.RBACAssignment, len(rows))
	for i, row := range rows {
		out[i] = rbacAssignmentToDomain(row)
	}
	return out, nil
}

// ListRBACAssignmentsForPrincipal returns enabled assignments for a user
// subject or any local directory departments supplied for that user.
func (r *configRepo) ListRBACAssignmentsForPrincipal(ctx context.Context, subject string, departmentKeys []string, limit int) ([]domain.RBACAssignment, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, fmt.Errorf("list rbac assignments: subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list rbac assignments: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	subjectPredicates := []predicate.RBACAssignment{
		rbacassignment.And(
			rbacassignment.SubjectKindEQ(string(domain.RBACSubjectKindUser)),
			rbacassignment.SubjectKeyEQ(subject),
		),
	}
	seenDepartmentKeys := map[string]struct{}{}
	for _, key := range departmentKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seenDepartmentKeys[key]; ok {
			continue
		}
		seenDepartmentKeys[key] = struct{}{}
		subjectPredicates = append(subjectPredicates, rbacassignment.And(
			rbacassignment.SubjectKindEQ(string(domain.RBACSubjectKindDepartment)),
			rbacassignment.SubjectKeyEQ(key),
		))
	}
	rows, err := r.tx.RBACAssignment.Query().
		Where(
			rbacassignment.EnabledEQ(true),
			rbacassignment.Or(subjectPredicates...),
		).
		Order(rbacassignment.ByUpdatedAt(entsql.OrderDesc()), rbacassignment.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rbac assignments: %w", err)
	}
	out := make([]domain.RBACAssignment, len(rows))
	for i, row := range rows {
		out[i] = rbacAssignmentToDomain(row)
	}
	return out, nil
}
