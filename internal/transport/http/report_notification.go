package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"strings"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportnotification"
)

// RetryReportNotification implements api.ServerInterface.
func (s *Server) RetryReportNotification(w stdhttp.ResponseWriter, r *stdhttp.Request, reportID int64) {
	if s.reportNotifier == nil {
		writeError(r.Context(), w, s.logger, stdhttp.StatusServiceUnavailable, "report notification retry is not configured", nil)
		return
	}
	if reportID <= 0 {
		writeError(r.Context(), w, s.logger, stdhttp.StatusBadRequest, "report_id must be positive", nil)
		return
	}
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionReportWorkflowManage) {
		return
	}
	body, err := decodeReportNotificationRetryRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, stdhttp.StatusBadRequest, err.Error(), nil)
		return
	}
	req, err := reportNotificationRetryRequest(reportID, body)
	if err != nil {
		writeError(r.Context(), w, s.logger, stdhttp.StatusBadRequest, err.Error(), nil)
		return
	}
	if req.NotificationPurpose == reportnotification.NotificationPurposeFinal {
		if err := s.ensureReportNotificationFinalReady(r.Context(), req.FinalReportID); err != nil {
			writeReportNotificationRetryError(r.Context(), w, s.logger, err)
			return
		}
	}
	result, err := s.reportNotifier.Send(r.Context(), req)
	if err != nil {
		writeReportNotificationRetryError(r.Context(), w, s.logger, err)
		return
	}
	writeJSON(r.Context(), w, s.logger, stdhttp.StatusOK, api.ReportNotificationRetryResponse{
		RetryState: reportNotificationRetryState(result.RetryState),
		Delivery:   reportNotificationDeliveryProof(result.Delivery),
	})
}

func decodeReportNotificationRetryRequest(w stdhttp.ResponseWriter, r *stdhttp.Request) (api.ReportNotificationRetryRequest, error) {
	var body api.ReportNotificationRetryRequest
	raw, err := readJSONRequestBody(w, r)
	if err != nil {
		return body, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return body, nil
	}
	if err := strictjson.Unmarshal(raw, &body); err != nil {
		return body, fmt.Errorf("invalid JSON request body: %w", err)
	}
	return body, nil
}

func reportNotificationRetryRequest(reportID int64, body api.ReportNotificationRetryRequest) (reportnotification.Request, error) {
	reportNotificationChannelProfileID, err := optionalNotificationChannelProfileID(body.ReportNotificationChannelProfileID)
	if err != nil {
		return reportnotification.Request{}, err
	}
	notificationPurpose, err := reportNotificationPurposeFromAPI(body.NotificationPurpose)
	if err != nil {
		return reportnotification.Request{}, err
	}
	return reportnotification.Request{
		FinalReportID:                      domain.FinalReportID(reportID),
		ReportNotificationChannelProfileID: reportNotificationChannelProfileID,
		NotificationPurpose:                notificationPurpose,
	}, nil
}

func (s *Server) ensureReportNotificationFinalReady(ctx context.Context, reportID domain.FinalReportID) error {
	var subReports []domain.SubReport
	var conclusions diagnosisConclusionBySnapshot
	var progress diagnosisProgressBySnapshot
	if err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		report, err := uow.Reports().FindFinalReportByID(ctx, reportID)
		if err != nil {
			return err
		}
		subReports, err = uow.Reports().ListSubReportsForFinalReport(ctx, report.ID, maxListLimit)
		if err != nil {
			return err
		}
		if len(subReports) == 0 {
			return fmt.Errorf("final report %d has no linked subreports: %w", reportID, domain.ErrPreconditionFailed)
		}
		var ferr error
		conclusions, progress, _, ferr = diagnosisStatesForSubReports(ctx, uow.Diagnosis(), subReports)
		return ferr
	}); err != nil {
		return err
	}
	for _, subReport := range subReports {
		if err := reportNotificationSubReportFinalReady(reportID, subReport, conclusions, progress); err != nil {
			return err
		}
	}
	return nil
}

func reportNotificationSubReportFinalReady(
	reportID domain.FinalReportID,
	subReport domain.SubReport,
	conclusions diagnosisConclusionBySnapshot,
	progress diagnosisProgressBySnapshot,
) error {
	snapshotID := subReport.EvidenceSnapshotID
	if snapshotID == 0 {
		return fmt.Errorf("final report %d subreport %d has no evidence snapshot: %w", reportID, subReport.ID, domain.ErrPreconditionFailed)
	}
	conclusion, hasConclusion := conclusions[snapshotID]
	if !reportNotificationConclusionConfirmed(conclusion, hasConclusion) {
		return fmt.Errorf(
			"final report %d evidence snapshot %d has no operator-confirmed diagnosis conclusion: %w",
			reportID,
			snapshotID,
			domain.ErrPreconditionFailed,
		)
	}
	latestProgress, hasProgress := progress[snapshotID]
	if reportProgressIsNewerThanConclusion(conclusion, hasConclusion, latestProgress, hasProgress) {
		return fmt.Errorf(
			"final report %d evidence snapshot %d has newer diagnosis progress after the confirmed conclusion: %w",
			reportID,
			snapshotID,
			domain.ErrPreconditionFailed,
		)
	}
	return nil
}

func reportFinalNotificationReadiness(
	reportID domain.FinalReportID,
	subReports []domain.SubReport,
	conclusions diagnosisConclusionBySnapshot,
	progress diagnosisProgressBySnapshot,
) api.ReportFinalNotificationReadiness {
	if len(subReports) == 0 {
		return blockedReportFinalNotificationReadiness(
			fmt.Sprintf("Final report %d has no linked subreports.", reportID),
		)
	}
	for _, subReport := range subReports {
		snapshotID := subReport.EvidenceSnapshotID
		if snapshotID == 0 {
			return blockedReportFinalNotificationReadiness(
				fmt.Sprintf("%s has no linked evidence snapshot.", reportNotificationSubReportLabel(subReport)),
			)
		}
		conclusion, hasConclusion := conclusions[snapshotID]
		if !reportNotificationConclusionConfirmed(conclusion, hasConclusion) {
			return blockedReportFinalNotificationReadiness(
				fmt.Sprintf("%s has no operator-confirmed AI conclusion yet.", reportNotificationSubReportLabel(subReport)),
			)
		}
		latestProgress, hasProgress := progress[snapshotID]
		if reportProgressIsNewerThanConclusion(conclusion, hasConclusion, latestProgress, hasProgress) {
			return blockedReportFinalNotificationReadiness(
				fmt.Sprintf("%s has newer diagnosis progress after the confirmed conclusion.", reportNotificationSubReportLabel(subReport)),
			)
		}
	}
	return api.ReportFinalNotificationReadiness{
		Ready:               true,
		NotificationPurpose: api.Final,
		Status:              string(api.ReportFinalNotificationReadinessStatusReady),
		StatusLabel:         "Final notification ready",
		Detail:              "All linked subreports have operator-confirmed AI conclusions; final notification can be sent.",
	}
}

func blockedReportFinalNotificationReadiness(detail string) api.ReportFinalNotificationReadiness {
	return api.ReportFinalNotificationReadiness{
		Ready:               false,
		NotificationPurpose: api.Handoff,
		Status:              string(api.ReportFinalNotificationReadinessStatusBlocked),
		StatusLabel:         "Final notification blocked",
		Detail:              detail,
	}
}

func reportNotificationSubReportLabel(subReport domain.SubReport) string {
	title := strings.TrimSpace(subReport.Title)
	if title != "" {
		return title
	}
	if subReport.ID != 0 {
		return fmt.Sprintf("Subreport %d", subReport.ID)
	}
	return "Linked subreport"
}

func reportNotificationConclusionConfirmed(conclusion api.DiagnosisRoomConclusionSummary, ok bool) bool {
	return ok && conclusion.ConfirmedBy != nil && strings.TrimSpace(*conclusion.ConfirmedBy) != ""
}

func reportNotificationPurposeFromAPI(in *api.ReportNotificationPurpose) (reportnotification.NotificationPurpose, error) {
	if in == nil {
		return "", nil
	}
	switch *in {
	case api.Handoff:
		return reportnotification.NotificationPurposeHandoff, nil
	case api.Final:
		return reportnotification.NotificationPurposeFinal, nil
	default:
		return "", fmt.Errorf("notification_purpose must be one of handoff or final")
	}
}

func reportNotificationDeliveryProof(delivery domain.ReportNotificationDelivery) api.ReportNotificationDeliveryProof {
	return reportNotificationDeliveryProofs([]domain.ReportNotificationDelivery{delivery})[0]
}

func reportNotificationRetryState(state reportnotification.RetryState) api.ReportNotificationRetryState {
	switch state {
	case reportnotification.RetryStateAlreadyDelivered:
		return api.ReportNotificationRetryStateAlreadyDelivered
	case reportnotification.RetryStateAlreadyPending:
		return api.ReportNotificationRetryStateAlreadyPending
	default:
		return api.ReportNotificationRetryStateSent
	}
}

func writeReportNotificationRetryError(ctx context.Context, w stdhttp.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(ctx, w, logger, stdhttp.StatusNotFound, "report not found", err)
	case errors.Is(err, domain.ErrPreconditionFailed):
		writeError(ctx, w, logger, stdhttp.StatusBadRequest, err.Error(), nil)
	case errors.Is(err, domain.ErrInvariantViolation):
		writeError(ctx, w, logger, stdhttp.StatusBadRequest, err.Error(), nil)
	default:
		writeError(ctx, w, logger, stdhttp.StatusInternalServerError, "retry report notification failed", err)
	}
}
