// Command report_ai_review_proof enriches a report live-smoke proof with
// sanitized diagnosis-room review evidence loaded from the local database.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/repository"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	maxProofBytes int64 = 10 * 1024 * 1024

	reportProofTaskLimit  = 5
	reportProofEventLimit = 100

	eventTurnPersisted             = "diagnosis_room.turn_persisted"
	eventEvidenceCollected         = "diagnosis_room.evidence_collected"
	eventFinalReady                = "diagnosis_room.final_conclusion_ready"
	eventFailed                    = "diagnosis_room.failed"
	eventClosed                    = "diagnosis_room.closed"
	eventSupplementalEvidence      = "diagnosis_room.supplemental_evidence_provided"
	eventCloseNotification         = "diagnosis_room.close_notification_sent"
	eventFinalReadyNotification    = "diagnosis_room.final_ready_notification_sent"
	eventAssistantTurnNotification = "diagnosis_room.assistant_turn_notification_sent"
)

type config struct {
	input          string
	output         string
	databaseURL    string
	listCandidates bool
	limit          int
	waitTimeout    time.Duration
	pollInterval   time.Duration
}

type smokeOutput struct {
	CheckedAt      string          `json:"checked_at"`
	Request        proofRequest    `json:"request"`
	Started        bool            `json:"started"`
	WorkflowID     string          `json:"workflow_id"`
	RunID          string          `json:"run_id"`
	Waited         bool            `json:"waited"`
	WorkflowResult *workflowResult `json:"workflow_result,omitempty"`
	Stats          replayStats     `json:"stats"`
	Snapshots      []snapshotRef   `json:"snapshots"`
	AIReview       *aiReviewProof  `json:"ai_review,omitempty"`
}

type proofRequest struct {
	PolicyID       int64  `json:"policy_id,omitempty"`
	WindowStart    string `json:"window_start"`
	WindowEnd      string `json:"window_end"`
	Limit          int    `json:"limit"`
	Scenario       string `json:"scenario"`
	CorrelationKey string `json:"correlation_key,omitempty"`
	WorkflowID     string `json:"workflow_id,omitempty"`
	Wait           bool   `json:"wait"`
	WaitTimeout    string `json:"wait_timeout,omitempty"`
}

type workflowResult struct {
	SubReportIDs               []int64 `json:"sub_report_ids"`
	FinalReportID              int64   `json:"final_report_id"`
	NotificationIdempotencyKey string  `json:"notification_idempotency_key"`
	ProviderMessageID          string  `json:"provider_message_id"`
	NotificationStatus         string  `json:"notification_status"`
}

type snapshotRef struct {
	ID         int64 `json:"id"`
	GroupIndex int   `json:"group_index"`
	EventCount int   `json:"event_count"`
}

type replayStats struct {
	Ingested           ingestStats `json:"ingested"`
	EventsLoaded       int         `json:"events_loaded"`
	GroupsBuilt        int         `json:"groups_built"`
	GroupsSaved        int         `json:"groups_saved"`
	GroupsRefreshed    int         `json:"groups_refreshed"`
	GroupsExisting     int         `json:"groups_existing"`
	SnapshotsSaved     int         `json:"snapshots_saved"`
	SnapshotsDuplicate int         `json:"snapshots_duplicate"`
	GroupsClosed       int         `json:"groups_closed"`
	Failed             int         `json:"failed"`
}

type ingestStats struct {
	Total     int `json:"total"`
	Saved     int `json:"saved"`
	Duplicate int `json:"duplicate"`
	Failed    int `json:"failed"`
}

type aiReviewProof struct {
	Status             string                     `json:"status"`
	ReviewedAt         string                     `json:"reviewed_at"`
	FinalReportID      int64                      `json:"final_report_id"`
	ReviewedSubReports []aiSubReportReview        `json:"reviewed_sub_reports"`
	PendingSubReports  []aiPendingSubReportReview `json:"pending_sub_reports,omitempty"`
}

type aiSubReportReview struct {
	SubReportID                       int64  `json:"sub_report_id"`
	EvidenceSnapshotID                int64  `json:"evidence_snapshot_id"`
	DiagnosisTaskID                   int64  `json:"diagnosis_task_id"`
	SessionID                         string `json:"session_id"`
	ChatSessionID                     int64  `json:"chat_session_id"`
	ConclusionStatus                  string `json:"conclusion_status"`
	ConclusionSource                  string `json:"conclusion_source"`
	Confidence                        string `json:"confidence"`
	RequiresHumanReview               *bool  `json:"requires_human_review"`
	ConfidenceTimelineCount           int    `json:"confidence_timeline_count"`
	EvidenceRequestCount              int    `json:"evidence_request_count"`
	MissingEvidenceRequestCount       int    `json:"missing_evidence_request_count"`
	EvidenceCollectionSuggestionCount int    `json:"evidence_collection_suggestion_count"`
	EvidenceCollectionResultCount     int    `json:"evidence_collection_result_count"`
	SupplementalEvidenceCount         int    `json:"supplemental_evidence_count"`
	NotificationTimelineCount         int    `json:"notification_timeline_count"`
}

type aiPendingSubReportReview struct {
	SubReportID        int64                `json:"sub_report_id"`
	EvidenceSnapshotID int64                `json:"evidence_snapshot_id"`
	Reason             string               `json:"reason"`
	TaskStates         []candidateTaskState `json:"task_states"`
}

type reportReader interface {
	FindFinalReportByID(ctx context.Context, id domain.FinalReportID) (domain.FinalReport, error)
	ListSubReportsForFinalReport(ctx context.Context, finalReportID domain.FinalReportID, limit int) ([]domain.SubReport, error)
}

type candidateReportReader interface {
	reportReader
	ListFinalReports(ctx context.Context, limit int) ([]domain.FinalReport, error)
}

type diagnosisReader interface {
	ListTasksByEvidenceSnapshot(ctx context.Context, snapshotID domain.EvidenceSnapshotID, limit int) ([]domain.DiagnosisTask, error)
	ListEventsByTaskAndKind(ctx context.Context, taskID domain.DiagnosisTaskID, kind string, limit int) ([]domain.DiagnosisTaskEvent, error)
}

type conclusionProof struct {
	TaskID              domain.DiagnosisTaskID
	SessionID           string
	ChatSessionID       int64
	Status              string
	Source              string
	Confidence          string
	RequiresHumanReview *bool
	OccurredAt          time.Time
}

type reviewCounts struct {
	ConfidenceTimeline            int
	EvidenceRequests              int
	MissingEvidenceRequests       int
	EvidenceCollectionSuggestions int
	EvidenceCollectionResult      int
	SupplementalEvidence          int
	NotificationTimeline          int
}

func (c reviewCounts) evidenceWorkCount() int {
	return c.EvidenceRequests +
		c.MissingEvidenceRequests +
		c.EvidenceCollectionSuggestions +
		c.EvidenceCollectionResult +
		c.SupplementalEvidence
}

type turnEventSummary struct {
	Confidence                        string
	RequiresHumanReview               *bool
	ConclusionStatus                  string
	EvidenceRequestCount              int
	MissingEvidenceRequestCount       int
	EvidenceCollectionSuggestionCount int
}

type candidateAudit struct {
	CheckedAt  string            `json:"checked_at"`
	Limit      int               `json:"limit"`
	Candidates []candidateReview `json:"candidates"`
}

type candidateReview struct {
	FinalReportID           int64              `json:"final_report_id"`
	SubReportCount          int                `json:"sub_report_count"`
	ReviewedSubReportCount  int                `json:"reviewed_sub_report_count"`
	AIReviewReady           bool               `json:"ai_review_ready"`
	MissingReviewedEvidence []candidateMissing `json:"missing_reviewed_evidence,omitempty"`
}

type candidateMissing struct {
	SubReportID        int64                `json:"sub_report_id"`
	EvidenceSnapshotID int64                `json:"evidence_snapshot_id"`
	Reason             string               `json:"reason"`
	TaskStates         []candidateTaskState `json:"task_states,omitempty"`
}

type candidateTaskState struct {
	DiagnosisTaskID                   int64  `json:"diagnosis_task_id"`
	TaskStatus                        string `json:"task_status,omitempty"`
	FailureReason                     string `json:"failure_reason,omitempty"`
	LatestTurnStatus                  string `json:"latest_turn_status,omitempty"`
	LatestConfidence                  string `json:"latest_confidence,omitempty"`
	LatestRequiresHumanReview         *bool  `json:"latest_requires_human_review,omitempty"`
	ConfidenceTimelineCount           int    `json:"confidence_timeline_count,omitempty"`
	EvidenceRequestCount              int    `json:"evidence_request_count,omitempty"`
	MissingEvidenceRequestCount       int    `json:"missing_evidence_request_count,omitempty"`
	EvidenceCollectionSuggestionCount int    `json:"evidence_collection_suggestion_count,omitempty"`
	EvidenceCollectionResultCount     int    `json:"evidence_collection_result_count,omitempty"`
	SupplementalEvidenceCount         int    `json:"supplemental_evidence_count,omitempty"`
	NotificationTimelineCount         int    `json:"notification_timeline_count,omitempty"`
	FinalReadyEventCount              int    `json:"final_ready_event_count,omitempty"`
	FailedEventCount                  int    `json:"failed_event_count,omitempty"`
	ClosedEventCount                  int    `json:"closed_event_count,omitempty"`
	LastEventKind                     string `json:"last_event_kind,omitempty"`
	LastEventAt                       string `json:"last_event_at,omitempty"`
}

func (s candidateTaskState) evidenceWorkCount() int {
	return s.EvidenceRequestCount +
		s.MissingEvidenceRequestCount +
		s.EvidenceCollectionSuggestionCount +
		s.EvidenceCollectionResultCount +
		s.SupplementalEvidenceCount
}

type conclusionEventPayload struct {
	Kind            string            `json:"kind"`
	SessionID       string            `json:"session_id,omitempty"`
	ChatSessionID   int64             `json:"chat_session_id,omitempty"`
	DiagnosisTaskID int64             `json:"diagnosis_task_id,omitempty"`
	FinalConclusion conclusionPayload `json:"final_conclusion"`
}

type conclusionPayload struct {
	Status              string `json:"status"`
	Source              string `json:"source"`
	EvidenceSnapshotID  int64  `json:"evidence_snapshot_id,omitempty"`
	Content             string `json:"content,omitempty"`
	Confidence          string `json:"confidence,omitempty"`
	RequiresHumanReview *bool  `json:"requires_human_review,omitempty"`
}

type turnEventPayload struct {
	Kind                string              `json:"kind"`
	DiagnosisTaskID     int64               `json:"diagnosis_task_id,omitempty"`
	Confidence          string              `json:"confidence,omitempty"`
	RequiresHumanReview *bool               `json:"requires_human_review,omitempty"`
	EvidenceRequests    []json.RawMessage   `json:"evidence_requests,omitempty"`
	ConsultationInsight consultationPayload `json:"consultation_insight,omitempty"`
}

type consultationPayload struct {
	MissingEvidenceRequests       []json.RawMessage `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []json.RawMessage `json:"evidence_collection_suggestions,omitempty"`
	ConclusionStatus              string            `json:"conclusion_status,omitempty"`
}

type evidenceCollectedPayload struct {
	Kind                      string            `json:"kind"`
	DiagnosisTaskID           int64             `json:"diagnosis_task_id,omitempty"`
	EvidenceCollectionResults []json.RawMessage `json:"evidence_collection_results,omitempty"`
}

type supplementalEvidencePayload struct {
	Kind                 string          `json:"kind"`
	DiagnosisTaskID      int64           `json:"diagnosis_task_id,omitempty"`
	SupplementalEvidence json.RawMessage `json:"supplemental_evidence"`
}

type notificationPayload struct {
	Kind            string `json:"kind"`
	DiagnosisTaskID int64  `json:"diagnosis_task_id,omitempty"`
	ProviderStatus  string `json:"provider_status,omitempty"`
}

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "[report-ai-review-proof] FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "[report-ai-review-proof] OK")
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	cfg, err := parseArgs(args, getenv)
	if err != nil {
		return err
	}

	client, err := repository.OpenPostgres(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer func() {
		if cerr := client.Close(); cerr != nil && stderr != nil {
			fmt.Fprintf(stderr, "[report-ai-review-proof] close postgres: %v\n", cerr)
		}
	}()

	factory := repository.NewFactory(client)
	if cfg.listCandidates {
		var audit candidateAudit
		err = factory.WithinTx(ctx, func(txCtx context.Context, uow ports.UnitOfWork) error {
			var aerr error
			audit, aerr = auditCandidates(txCtx, uow.Reports(), uow.Diagnosis(), cfg.limit, nowUTC())
			return aerr
		})
		if err != nil {
			return err
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(audit); err != nil {
			return fmt.Errorf("write candidate audit: %w", err)
		}
		return nil
	}

	raw, err := readProofFile(cfg.input)
	if err != nil {
		return err
	}
	var proof smokeOutput
	if err := strictjson.Unmarshal(raw, &proof); err != nil {
		return fmt.Errorf("decode report smoke proof: %w", err)
	}

	enriched, err := attachAIReviewWithRetry(ctx, cfg.waitTimeout, cfg.pollInterval, func(attemptCtx context.Context, reviewedAt time.Time) (smokeOutput, error) {
		var enriched smokeOutput
		err := factory.WithinTx(attemptCtx, func(txCtx context.Context, uow ports.UnitOfWork) error {
			var berr error
			enriched, berr = attachAIReview(txCtx, uow.Reports(), uow.Diagnosis(), proof, reviewedAt)
			return berr
		})
		return enriched, err
	})
	if err != nil {
		return err
	}
	if err := writeProofFile(cfg.output, enriched); err != nil {
		return err
	}
	return nil
}

func parseArgs(args []string, getenv func(string) string) (config, error) {
	var cfg config
	var rawWaitTimeout, rawPollInterval string
	fs := flag.NewFlagSet("report_ai_review_proof", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.input, "input", "", "report live-smoke JSON proof to enrich")
	fs.StringVar(&cfg.output, "output", "", "output JSON proof path; defaults to --input")
	fs.StringVar(&cfg.databaseURL, "database-url", "", "PostgreSQL DATABASE_URL; defaults to env DATABASE_URL")
	fs.BoolVar(&cfg.listCandidates, "list-candidates", false, "list recent final reports and whether diagnosis-room AI review proof is available")
	fs.IntVar(&cfg.limit, "limit", 20, "candidate final report limit when --list-candidates is set")
	fs.StringVar(&rawWaitTimeout, "wait-timeout", "", "optional duration to wait for diagnosis-room AI review evidence")
	fs.StringVar(&rawPollInterval, "poll-interval", "", "poll interval while --wait-timeout is active")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("usage: report_ai_review_proof --input <output.json> [--output <output.json>] [--database-url <dsn>] OR report_ai_review_proof --list-candidates [--limit N]")
	}
	cfg.input = strings.TrimSpace(cfg.input)
	cfg.output = strings.TrimSpace(cfg.output)
	if cfg.listCandidates {
		if cfg.input != "" || cfg.output != "" {
			return config{}, fmt.Errorf("--input and --output must be omitted when --list-candidates is set")
		}
		if strings.TrimSpace(rawWaitTimeout) != "" || strings.TrimSpace(rawPollInterval) != "" {
			return config{}, fmt.Errorf("--wait-timeout and --poll-interval must be omitted when --list-candidates is set")
		}
		if cfg.limit <= 0 {
			return config{}, fmt.Errorf("--limit must be > 0")
		}
	} else if cfg.input == "" {
		return config{}, fmt.Errorf("--input is required")
	}
	if cfg.output == "" {
		cfg.output = cfg.input
	}
	if !cfg.listCandidates {
		if rawWaitTimeout == "" && getenv != nil {
			rawWaitTimeout = strings.TrimSpace(getenv("REPORT_LIVE_SMOKE_AI_REVIEW_WAIT_TIMEOUT"))
		}
		if rawPollInterval == "" && getenv != nil {
			rawPollInterval = strings.TrimSpace(getenv("REPORT_LIVE_SMOKE_AI_REVIEW_POLL_INTERVAL"))
		}
		waitTimeout, err := optionalDuration("wait-timeout", rawWaitTimeout)
		if err != nil {
			return config{}, err
		}
		pollInterval, err := optionalDuration("poll-interval", rawPollInterval)
		if err != nil {
			return config{}, err
		}
		if waitTimeout > 0 && pollInterval == 0 {
			pollInterval = 5 * time.Second
		}
		if waitTimeout == 0 && pollInterval > 0 {
			return config{}, fmt.Errorf("--poll-interval requires --wait-timeout")
		}
		cfg.waitTimeout = waitTimeout
		cfg.pollInterval = pollInterval
	}
	cfg.databaseURL = strings.TrimSpace(cfg.databaseURL)
	if cfg.databaseURL == "" && getenv != nil {
		cfg.databaseURL = strings.TrimSpace(getenv("DATABASE_URL"))
	}
	if cfg.databaseURL == "" {
		return config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func optionalDuration(label, raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("--%s must be a valid duration: %w", label, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("--%s must be > 0", label)
	}
	return value, nil
}

func attachAIReviewWithRetry(
	ctx context.Context,
	waitTimeout time.Duration,
	pollInterval time.Duration,
	attach func(context.Context, time.Time) (smokeOutput, error),
) (smokeOutput, error) {
	if attach == nil {
		return smokeOutput{}, fmt.Errorf("attach function must be configured")
	}
	if waitTimeout == 0 {
		return attach(ctx, nowUTC())
	}
	if pollInterval <= 0 {
		return smokeOutput{}, fmt.Errorf("poll interval must be > 0 when wait timeout is set")
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		out, err := attach(waitCtx, nowUTC())
		if err == nil {
			return out, nil
		}
		lastErr = err
		select {
		case <-waitCtx.Done():
			return smokeOutput{}, fmt.Errorf("wait for AI review proof: %w", errors.Join(waitCtx.Err(), lastErr))
		case <-ticker.C:
		}
	}
}

func auditCandidates(
	ctx context.Context,
	reports candidateReportReader,
	diagnosis diagnosisReader,
	limit int,
	checkedAt time.Time,
) (candidateAudit, error) {
	if reports == nil {
		return candidateAudit{}, fmt.Errorf("report reader must be configured")
	}
	if diagnosis == nil {
		return candidateAudit{}, fmt.Errorf("diagnosis reader must be configured")
	}
	if limit <= 0 {
		return candidateAudit{}, fmt.Errorf("candidate limit must be > 0")
	}
	finalReports, err := reports.ListFinalReports(ctx, limit)
	if err != nil {
		return candidateAudit{}, fmt.Errorf("list final reports: %w", err)
	}
	out := candidateAudit{
		CheckedAt:  checkedAt.UTC().Format(time.RFC3339Nano),
		Limit:      limit,
		Candidates: make([]candidateReview, 0, len(finalReports)),
	}
	for _, report := range finalReports {
		subReports, err := reports.ListSubReportsForFinalReport(ctx, report.ID, reportProofEventLimit)
		if err != nil {
			return candidateAudit{}, fmt.Errorf("list subreports for final report %d: %w", report.ID, err)
		}
		item := candidateReview{
			FinalReportID:  int64(report.ID),
			SubReportCount: len(subReports),
		}
		for _, subReport := range subReports {
			_, err := buildSubReportReview(ctx, diagnosis, subReport, subReport.EvidenceSnapshotID)
			if err != nil {
				taskStates, stateErr := taskStatesForSnapshot(ctx, diagnosis, subReport.EvidenceSnapshotID)
				reason := sanitizeReason(err)
				if stateErr != nil {
					reason += "; task state unavailable: " + sanitizeReason(stateErr)
				}
				item.MissingReviewedEvidence = append(item.MissingReviewedEvidence, candidateMissing{
					SubReportID:        int64(subReport.ID),
					EvidenceSnapshotID: int64(subReport.EvidenceSnapshotID),
					Reason:             reason,
					TaskStates:         taskStates,
				})
				continue
			}
			item.ReviewedSubReportCount++
		}
		item.AIReviewReady = item.SubReportCount > 0 && item.ReviewedSubReportCount == item.SubReportCount
		out.Candidates = append(out.Candidates, item)
	}
	return out, nil
}

func taskStatesForSnapshot(
	ctx context.Context,
	diagnosis diagnosisReader,
	snapshotID domain.EvidenceSnapshotID,
) ([]candidateTaskState, error) {
	tasks, err := diagnosis.ListTasksByEvidenceSnapshot(ctx, snapshotID, reportProofTaskLimit)
	if err != nil {
		return nil, fmt.Errorf("list diagnosis tasks for evidence snapshot %d: %w", snapshotID, err)
	}
	if len(tasks) == 0 {
		return nil, nil
	}
	out := make([]candidateTaskState, 0, len(tasks))
	for _, task := range tasks {
		state, err := taskStateForCandidate(ctx, diagnosis, task)
		if err != nil {
			return nil, err
		}
		out = append(out, state)
	}
	return out, nil
}

func taskStateForCandidate(
	ctx context.Context,
	diagnosis diagnosisReader,
	task domain.DiagnosisTask,
) (candidateTaskState, error) {
	counts, err := diagnosisReviewCounts(ctx, diagnosis, task.ID)
	if err != nil {
		return candidateTaskState{}, err
	}
	state := candidateTaskState{
		DiagnosisTaskID:                   int64(task.ID),
		TaskStatus:                        string(task.Status),
		FailureReason:                     sanitizeOptionalText(task.FailureReason),
		ConfidenceTimelineCount:           counts.ConfidenceTimeline,
		EvidenceRequestCount:              counts.EvidenceRequests,
		MissingEvidenceRequestCount:       counts.MissingEvidenceRequests,
		EvidenceCollectionSuggestionCount: counts.EvidenceCollectionSuggestions,
		EvidenceCollectionResultCount:     counts.EvidenceCollectionResult,
		SupplementalEvidenceCount:         counts.SupplementalEvidence,
		NotificationTimelineCount:         counts.NotificationTimeline,
	}
	turns, err := diagnosis.ListEventsByTaskAndKind(ctx, task.ID, eventTurnPersisted, reportProofEventLimit)
	if err != nil {
		return candidateTaskState{}, fmt.Errorf("list diagnosis turn events for task %d: %w", task.ID, err)
	}
	for _, event := range turns {
		summary, err := parseTurnEvent(event)
		if err != nil {
			return candidateTaskState{}, err
		}
		updateLatestTaskEvent(&state, event)
		if summary.ConclusionStatus != "" {
			state.LatestTurnStatus = summary.ConclusionStatus
		}
		if validConfidence(summary.Confidence) {
			state.LatestConfidence = strings.TrimSpace(summary.Confidence)
		}
		if summary.RequiresHumanReview != nil {
			state.LatestRequiresHumanReview = summary.RequiresHumanReview
		}
	}
	finalReady, err := diagnosis.ListEventsByTaskAndKind(ctx, task.ID, eventFinalReady, reportProofEventLimit)
	if err != nil {
		return candidateTaskState{}, fmt.Errorf("list diagnosis final-ready events for task %d: %w", task.ID, err)
	}
	state.FinalReadyEventCount = len(finalReady)
	for _, event := range finalReady {
		updateLatestTaskEvent(&state, event)
	}
	failed, err := diagnosis.ListEventsByTaskAndKind(ctx, task.ID, eventFailed, reportProofEventLimit)
	if err != nil {
		return candidateTaskState{}, fmt.Errorf("list diagnosis failed events for task %d: %w", task.ID, err)
	}
	state.FailedEventCount = len(failed)
	for _, event := range failed {
		updateLatestTaskEvent(&state, event)
	}
	closed, err := diagnosis.ListEventsByTaskAndKind(ctx, task.ID, eventClosed, reportProofEventLimit)
	if err != nil {
		return candidateTaskState{}, fmt.Errorf("list diagnosis closed events for task %d: %w", task.ID, err)
	}
	state.ClosedEventCount = len(closed)
	for _, event := range closed {
		updateLatestTaskEvent(&state, event)
	}
	return state, nil
}

func updateLatestTaskEvent(state *candidateTaskState, event domain.DiagnosisTaskEvent) {
	if state == nil {
		return
	}
	occurred := eventOccurrence(event)
	if state.LastEventAt != "" {
		last, err := time.Parse(time.RFC3339Nano, state.LastEventAt)
		if err == nil && !occurred.After(last) {
			return
		}
	}
	state.LastEventKind = event.Kind
	state.LastEventAt = occurred.UTC().Format(time.RFC3339Nano)
}

func sanitizeReason(err error) string {
	return sanitizeText(err.Error())
}

func sanitizeOptionalText(raw string) string {
	value := sanitizeText(raw)
	if value == "unknown" {
		return ""
	}
	return value
}

func sanitizeText(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "unknown"
	}
	value = strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', '\t':
			return ' '
		default:
			return r
		}
	}, value)
	fields := strings.Fields(value)
	value = strings.Join(fields, " ")
	if len(value) > 240 {
		return value[:240]
	}
	return value
}

func attachAIReview(
	ctx context.Context,
	reports reportReader,
	diagnosis diagnosisReader,
	proof smokeOutput,
	reviewedAt time.Time,
) (smokeOutput, error) {
	if reports == nil {
		return smokeOutput{}, fmt.Errorf("report reader must be configured")
	}
	if diagnosis == nil {
		return smokeOutput{}, fmt.Errorf("diagnosis reader must be configured")
	}
	if proof.WorkflowResult == nil {
		return smokeOutput{}, fmt.Errorf("workflow_result must be present")
	}
	if proof.WorkflowResult.FinalReportID <= 0 {
		return smokeOutput{}, fmt.Errorf("workflow_result.final_report_id must be > 0")
	}
	if len(proof.WorkflowResult.SubReportIDs) == 0 {
		return smokeOutput{}, fmt.Errorf("workflow_result.sub_report_ids must be non-empty")
	}
	if len(proof.WorkflowResult.SubReportIDs) != len(proof.Snapshots) {
		return smokeOutput{}, fmt.Errorf("workflow_result.sub_report_ids length must match snapshots length")
	}

	finalReportID := domain.FinalReportID(proof.WorkflowResult.FinalReportID)
	if _, err := reports.FindFinalReportByID(ctx, finalReportID); err != nil {
		return smokeOutput{}, fmt.Errorf("find final report %d: %w", finalReportID, err)
	}
	subReports, err := reports.ListSubReportsForFinalReport(ctx, finalReportID, len(proof.WorkflowResult.SubReportIDs)+reportProofTaskLimit)
	if err != nil {
		return smokeOutput{}, fmt.Errorf("list subreports for final report %d: %w", finalReportID, err)
	}
	subReportsByID := make(map[domain.SubReportID]domain.SubReport, len(subReports))
	for _, subReport := range subReports {
		subReportsByID[subReport.ID] = subReport
	}

	review := aiReviewProof{
		Status:             "complete",
		ReviewedAt:         reviewedAt.UTC().Format(time.RFC3339Nano),
		FinalReportID:      int64(finalReportID),
		ReviewedSubReports: make([]aiSubReportReview, 0, len(proof.WorkflowResult.SubReportIDs)),
	}
	for i, rawSubReportID := range proof.WorkflowResult.SubReportIDs {
		subReportID := domain.SubReportID(rawSubReportID)
		subReport, ok := subReportsByID[subReportID]
		if !ok {
			return smokeOutput{}, fmt.Errorf("subreport %d is not linked to final report %d", subReportID, finalReportID)
		}
		snapshotID := domain.EvidenceSnapshotID(proof.Snapshots[i].ID)
		if subReport.EvidenceSnapshotID != snapshotID {
			return smokeOutput{}, fmt.Errorf("subreport %d evidence_snapshot_id = %d, want snapshot %d", subReportID, subReport.EvidenceSnapshotID, snapshotID)
		}
		item, err := buildSubReportReview(ctx, diagnosis, subReport, snapshotID)
		if err != nil {
			pending, pendingErr := buildPendingSubReportReview(ctx, diagnosis, subReport, snapshotID, err)
			if pendingErr != nil {
				return smokeOutput{}, pendingErr
			}
			review.PendingSubReports = append(review.PendingSubReports, pending)
			continue
		}
		review.ReviewedSubReports = append(review.ReviewedSubReports, item)
	}
	if len(review.PendingSubReports) > 0 {
		review.Status = "pending_evidence"
	}
	proof.AIReview = &review
	return proof, nil
}

func buildPendingSubReportReview(
	ctx context.Context,
	diagnosis diagnosisReader,
	subReport domain.SubReport,
	snapshotID domain.EvidenceSnapshotID,
	cause error,
) (aiPendingSubReportReview, error) {
	if cause == nil || !strings.Contains(cause.Error(), "has no available diagnosis conclusion") {
		return aiPendingSubReportReview{}, cause
	}
	taskStates, err := taskStatesForSnapshot(ctx, diagnosis, snapshotID)
	if err != nil {
		return aiPendingSubReportReview{}, fmt.Errorf("build pending AI review for subreport %d: %w", subReport.ID, err)
	}
	if len(taskStates) == 0 {
		return aiPendingSubReportReview{}, cause
	}
	if !hasReadyPendingTaskState(taskStates) {
		return aiPendingSubReportReview{}, fmt.Errorf("%w; pending diagnosis state is not ready", cause)
	}
	return aiPendingSubReportReview{
		SubReportID:        int64(subReport.ID),
		EvidenceSnapshotID: int64(snapshotID),
		Reason:             sanitizeReason(cause),
		TaskStates:         taskStates,
	}, nil
}

func hasReadyPendingTaskState(states []candidateTaskState) bool {
	for _, state := range states {
		if hasTerminalFailedTaskState(state) {
			return true
		}
		switch strings.TrimSpace(state.LatestTurnStatus) {
		case "investigating", "needs_evidence", "ready_for_review":
		default:
			continue
		}
		if !validConfidence(state.LatestConfidence) || state.LatestRequiresHumanReview == nil {
			continue
		}
		if state.ConfidenceTimelineCount <= 0 || state.NotificationTimelineCount <= 0 {
			continue
		}
		if state.evidenceWorkCount() == 0 {
			continue
		}
		if state.FinalReadyEventCount != 0 || state.ClosedEventCount != 0 {
			continue
		}
		return true
	}
	return false
}

func hasTerminalFailedTaskState(state candidateTaskState) bool {
	if strings.TrimSpace(state.TaskStatus) != string(domain.DiagnosisStatusFailed) {
		return false
	}
	if strings.TrimSpace(state.FailureReason) == "" || state.FailedEventCount <= 0 {
		return false
	}
	return strings.TrimSpace(state.LastEventKind) != "" && strings.TrimSpace(state.LastEventAt) != ""
}

func buildSubReportReview(
	ctx context.Context,
	diagnosis diagnosisReader,
	subReport domain.SubReport,
	snapshotID domain.EvidenceSnapshotID,
) (aiSubReportReview, error) {
	tasks, err := diagnosis.ListTasksByEvidenceSnapshot(ctx, snapshotID, reportProofTaskLimit)
	if err != nil {
		return aiSubReportReview{}, fmt.Errorf("list diagnosis tasks for evidence snapshot %d: %w", snapshotID, err)
	}
	if len(tasks) == 0 {
		return aiSubReportReview{}, fmt.Errorf("subreport %d evidence snapshot %d has no diagnosis tasks", subReport.ID, snapshotID)
	}
	var best conclusionProof
	bestSet := false
	for _, task := range tasks {
		conclusion, ok, err := latestConclusion(ctx, diagnosis, task, snapshotID)
		if err != nil {
			return aiSubReportReview{}, err
		}
		if !ok {
			continue
		}
		if !bestSet || conclusion.OccurredAt.After(best.OccurredAt) {
			best = conclusion
			bestSet = true
		}
	}
	if !bestSet {
		return aiSubReportReview{}, fmt.Errorf("subreport %d evidence snapshot %d has no available diagnosis conclusion", subReport.ID, snapshotID)
	}
	counts, err := diagnosisReviewCounts(ctx, diagnosis, best.TaskID)
	if err != nil {
		return aiSubReportReview{}, err
	}
	if counts.evidenceWorkCount() == 0 {
		return aiSubReportReview{}, fmt.Errorf("subreport %d diagnosis task %d has no evidence guidance, collection result, or supplemental evidence", subReport.ID, best.TaskID)
	}
	return aiSubReportReview{
		SubReportID:                       int64(subReport.ID),
		EvidenceSnapshotID:                int64(snapshotID),
		DiagnosisTaskID:                   int64(best.TaskID),
		SessionID:                         best.SessionID,
		ChatSessionID:                     best.ChatSessionID,
		ConclusionStatus:                  best.Status,
		ConclusionSource:                  best.Source,
		Confidence:                        best.Confidence,
		RequiresHumanReview:               best.RequiresHumanReview,
		ConfidenceTimelineCount:           counts.ConfidenceTimeline,
		EvidenceRequestCount:              counts.EvidenceRequests,
		MissingEvidenceRequestCount:       counts.MissingEvidenceRequests,
		EvidenceCollectionSuggestionCount: counts.EvidenceCollectionSuggestions,
		EvidenceCollectionResultCount:     counts.EvidenceCollectionResult,
		SupplementalEvidenceCount:         counts.SupplementalEvidence,
		NotificationTimelineCount:         counts.NotificationTimeline,
	}, nil
}

func latestConclusion(
	ctx context.Context,
	diagnosis diagnosisReader,
	task domain.DiagnosisTask,
	snapshotID domain.EvidenceSnapshotID,
) (conclusionProof, bool, error) {
	var best conclusionProof
	bestSet := false
	for _, kind := range []string{eventFinalReady, eventClosed} {
		events, err := diagnosis.ListEventsByTaskAndKind(ctx, task.ID, kind, reportProofEventLimit)
		if err != nil {
			return conclusionProof{}, false, fmt.Errorf("list diagnosis %s events for task %d: %w", kind, task.ID, err)
		}
		for _, event := range events {
			item, ok, err := parseConclusionEvent(event, snapshotID)
			if err != nil {
				return conclusionProof{}, false, err
			}
			if !ok {
				continue
			}
			if !bestSet || item.OccurredAt.After(best.OccurredAt) {
				best = item
				bestSet = true
			}
		}
	}
	return best, bestSet, nil
}

func parseConclusionEvent(event domain.DiagnosisTaskEvent, snapshotID domain.EvidenceSnapshotID) (conclusionProof, bool, error) {
	if len(event.Payload) == 0 {
		return conclusionProof{}, false, fmt.Errorf("diagnosis conclusion event %d has empty payload", event.ID)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return conclusionProof{}, false, fmt.Errorf("diagnosis conclusion event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload conclusionEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return conclusionProof{}, false, fmt.Errorf("diagnosis conclusion event %d payload: %w", event.ID, err)
	}
	if err := validateEventEnvelope(event, payload.Kind, payload.DiagnosisTaskID); err != nil {
		return conclusionProof{}, false, err
	}
	final := payload.FinalConclusion
	if final.Status != "available" {
		return conclusionProof{}, false, nil
	}
	if final.EvidenceSnapshotID != int64(snapshotID) {
		return conclusionProof{}, false, nil
	}
	if strings.TrimSpace(final.Content) == "" ||
		strings.TrimSpace(payload.SessionID) == "" ||
		payload.ChatSessionID <= 0 ||
		strings.TrimSpace(final.Source) == "" ||
		!validConfidence(final.Confidence) ||
		final.RequiresHumanReview == nil {
		return conclusionProof{}, false, nil
	}
	return conclusionProof{
		TaskID:              event.TaskID,
		SessionID:           strings.TrimSpace(payload.SessionID),
		ChatSessionID:       payload.ChatSessionID,
		Status:              final.Status,
		Source:              strings.TrimSpace(final.Source),
		Confidence:          strings.TrimSpace(final.Confidence),
		RequiresHumanReview: final.RequiresHumanReview,
		OccurredAt:          eventOccurrence(event),
	}, true, nil
}

func diagnosisReviewCounts(ctx context.Context, diagnosis diagnosisReader, taskID domain.DiagnosisTaskID) (reviewCounts, error) {
	var counts reviewCounts
	turnEvents, err := diagnosis.ListEventsByTaskAndKind(ctx, taskID, eventTurnPersisted, reportProofEventLimit)
	if err != nil {
		return reviewCounts{}, fmt.Errorf("list diagnosis turn events for task %d: %w", taskID, err)
	}
	for _, event := range turnEvents {
		summary, err := parseTurnEvent(event)
		if err != nil {
			return reviewCounts{}, err
		}
		if validConfidence(summary.Confidence) {
			counts.ConfidenceTimeline++
		}
		counts.EvidenceRequests += summary.EvidenceRequestCount
		counts.MissingEvidenceRequests += summary.MissingEvidenceRequestCount
		counts.EvidenceCollectionSuggestions += summary.EvidenceCollectionSuggestionCount
	}

	collectedEvents, err := diagnosis.ListEventsByTaskAndKind(ctx, taskID, eventEvidenceCollected, reportProofEventLimit)
	if err != nil {
		return reviewCounts{}, fmt.Errorf("list diagnosis evidence events for task %d: %w", taskID, err)
	}
	for _, event := range collectedEvents {
		n, err := parseEvidenceCollectedEvent(event)
		if err != nil {
			return reviewCounts{}, err
		}
		counts.EvidenceCollectionResult += n
	}

	supplementalEvents, err := diagnosis.ListEventsByTaskAndKind(ctx, taskID, eventSupplementalEvidence, reportProofEventLimit)
	if err != nil {
		return reviewCounts{}, fmt.Errorf("list diagnosis supplemental evidence events for task %d: %w", taskID, err)
	}
	for _, event := range supplementalEvents {
		ok, err := parseSupplementalEvidenceEvent(event)
		if err != nil {
			return reviewCounts{}, err
		}
		if ok {
			counts.SupplementalEvidence++
		}
	}

	for _, kind := range []string{eventAssistantTurnNotification, eventFinalReadyNotification, eventCloseNotification} {
		events, err := diagnosis.ListEventsByTaskAndKind(ctx, taskID, kind, reportProofEventLimit)
		if err != nil {
			return reviewCounts{}, fmt.Errorf("list diagnosis notification events for task %d: %w", taskID, err)
		}
		for _, event := range events {
			ok, err := parseNotificationEvent(event)
			if err != nil {
				return reviewCounts{}, err
			}
			if ok {
				counts.NotificationTimeline++
			}
		}
	}
	return counts, nil
}

func parseTurnEvent(event domain.DiagnosisTaskEvent) (turnEventSummary, error) {
	if len(event.Payload) == 0 {
		return turnEventSummary{}, fmt.Errorf("diagnosis turn event %d has empty payload", event.ID)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return turnEventSummary{}, fmt.Errorf("diagnosis turn event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload turnEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return turnEventSummary{}, fmt.Errorf("diagnosis turn event %d payload: %w", event.ID, err)
	}
	if err := validateEventEnvelope(event, payload.Kind, payload.DiagnosisTaskID); err != nil {
		return turnEventSummary{}, err
	}
	return turnEventSummary{
		Confidence:                        strings.TrimSpace(payload.Confidence),
		RequiresHumanReview:               payload.RequiresHumanReview,
		ConclusionStatus:                  strings.TrimSpace(payload.ConsultationInsight.ConclusionStatus),
		EvidenceRequestCount:              len(payload.EvidenceRequests),
		MissingEvidenceRequestCount:       len(payload.ConsultationInsight.MissingEvidenceRequests),
		EvidenceCollectionSuggestionCount: len(payload.ConsultationInsight.EvidenceCollectionSuggestions),
	}, nil
}

func parseEvidenceCollectedEvent(event domain.DiagnosisTaskEvent) (int, error) {
	if len(event.Payload) == 0 {
		return 0, fmt.Errorf("diagnosis evidence event %d has empty payload", event.ID)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return 0, fmt.Errorf("diagnosis evidence event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload evidenceCollectedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return 0, fmt.Errorf("diagnosis evidence event %d payload: %w", event.ID, err)
	}
	if err := validateEventEnvelope(event, payload.Kind, payload.DiagnosisTaskID); err != nil {
		return 0, err
	}
	return len(payload.EvidenceCollectionResults), nil
}

func parseSupplementalEvidenceEvent(event domain.DiagnosisTaskEvent) (bool, error) {
	if len(event.Payload) == 0 {
		return false, fmt.Errorf("diagnosis supplemental evidence event %d has empty payload", event.ID)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return false, fmt.Errorf("diagnosis supplemental evidence event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload supplementalEvidencePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return false, fmt.Errorf("diagnosis supplemental evidence event %d payload: %w", event.ID, err)
	}
	if err := validateEventEnvelope(event, payload.Kind, payload.DiagnosisTaskID); err != nil {
		return false, err
	}
	return len(payload.SupplementalEvidence) > 0, nil
}

func parseNotificationEvent(event domain.DiagnosisTaskEvent) (bool, error) {
	if len(event.Payload) == 0 {
		return false, fmt.Errorf("diagnosis notification event %d has empty payload", event.ID)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return false, fmt.Errorf("diagnosis notification event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload notificationPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return false, fmt.Errorf("diagnosis notification event %d payload: %w", event.ID, err)
	}
	if err := validateEventEnvelope(event, payload.Kind, payload.DiagnosisTaskID); err != nil {
		return false, err
	}
	switch strings.TrimSpace(payload.ProviderStatus) {
	case "accepted", "delivered":
		return true, nil
	default:
		return false, nil
	}
}

func validateEventEnvelope(event domain.DiagnosisTaskEvent, payloadKind string, payloadTaskID int64) error {
	if payloadKind != "" && payloadKind != event.Kind {
		return fmt.Errorf("diagnosis event %d kind mismatch", event.ID)
	}
	if payloadTaskID != 0 && domain.DiagnosisTaskID(payloadTaskID) != event.TaskID {
		return fmt.Errorf("diagnosis event %d task mismatch", event.ID)
	}
	return nil
}

func validConfidence(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

func eventOccurrence(event domain.DiagnosisTaskEvent) time.Time {
	if !event.OccurredAt.IsZero() {
		return event.OccurredAt
	}
	return event.RecordedAt
}

func readProofFile(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean); err != nil {
		return nil, err
	}
	// #nosec G304,G703 -- this manual proof helper opens the operator-supplied JSON path.
	f, err := os.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", clean, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxProofBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", clean, err)
	}
	if int64(len(raw)) > maxProofBytes {
		return nil, fmt.Errorf("%s exceeds maximum proof size %d bytes", clean, maxProofBytes)
	}
	return raw, nil
}

func requireRegularFile(path string) error {
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("stat %s: %w", clean, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular file, not a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", clean)
	}
	return nil
}

func writeProofFile(path string, proof smokeOutput) error {
	clean := filepath.Clean(path)
	if info, err := os.Lstat(clean); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s must not be a symlink", clean)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s must be a regular file", clean)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", clean, err)
	}
	dir := filepath.Dir(clean)
	base := filepath.Base(clean)
	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary proof file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpName)
		}
	}()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(proof); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode proof %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close proof %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, clean); err != nil {
		return fmt.Errorf("replace proof %s: %w", clean, err)
	}
	removeTmp = false
	return nil
}
