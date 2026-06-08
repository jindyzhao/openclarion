package repository

import (
	"context"
	"fmt"
	"sync/atomic"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertsourceprofile"
	"github.com/openclarion/openclarion/internal/persistence/ent/groupingpolicy"
	"github.com/openclarion/openclarion/internal/persistence/ent/notificationchannelprofile"
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
	return notificationChannelProfileToDomain(row), nil
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
	}
	return out, nil
}
