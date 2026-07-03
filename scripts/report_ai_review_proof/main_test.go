package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestAttachAIReviewBuildsSanitizedProof(t *testing.T) {
	reviewedAt := time.Date(2026, 5, 29, 1, 4, 0, 0, time.UTC)
	proof, err := attachAIReview(
		context.Background(),
		fakeReportReader{
			finalReports: map[domain.FinalReportID]domain.FinalReport{
				11: {ID: 11},
			},
			subReports: map[domain.FinalReportID][]domain.SubReport{
				11: {testSubReport(21, 7)},
			},
		},
		fakeDiagnosisReader{
			tasks: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				7: {testDiagnosisTask(31, 7)},
			},
			events: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				31: {
					eventFinalReady: {
						testEvent(101, 31, eventFinalReady, `{
							"kind":"diagnosis_room.final_conclusion_ready",
							"session_id":"diagnosis-session-31",
							"chat_session_id":51,
							"diagnosis_task_id":31,
							"final_conclusion":{
								"status":"available",
								"source":"latest_assistant_turn",
								"evidence_snapshot_id":7,
								"content":"Final diagnosis retained in the database.",
								"confidence":"high",
								"requires_human_review":true
							}
						}`),
					},
					eventTurnPersisted: {
						testEvent(102, 31, eventTurnPersisted, `{
							"kind":"diagnosis_room.turn_persisted",
							"diagnosis_task_id":31,
							"confidence":"medium",
							"evidence_requests":[{"tool":"active_alerts","reason":"Check sibling alerts."}],
							"consultation_insight":{
								"missing_evidence_requests":[{"label":"pod logs","detail":"Need recent pod logs.","priority":"high"}],
								"evidence_collection_suggestions":[{"tool":"prometheus","query":"up"}]
							}
						}`),
					},
					eventEvidenceCollected: {
						testEvent(103, 31, eventEvidenceCollected, `{
							"kind":"diagnosis_room.evidence_collected",
							"diagnosis_task_id":31,
							"evidence_collection_results":[{"tool":"active_alerts","status":"collected"}]
						}`),
					},
					eventAssistantTurnNotification: {
						testEvent(104, 31, eventAssistantTurnNotification, `{
							"kind":"diagnosis_room.assistant_turn_notification_sent",
							"diagnosis_task_id":31,
							"provider_status":"accepted"
						}`),
					},
				},
			},
		},
		testSmokeProof(),
		reviewedAt,
	)
	if err != nil {
		t.Fatalf("attachAIReview: %v", err)
	}
	if proof.AIReview == nil {
		t.Fatal("AIReview is nil")
	}
	review := proof.AIReview
	if review.Status != "complete" ||
		review.ReviewedAt != reviewedAt.Format(time.RFC3339Nano) ||
		review.FinalReportID != 11 ||
		len(review.ReviewedSubReports) != 1 {
		t.Fatalf("AIReview = %+v", review)
	}
	item := review.ReviewedSubReports[0]
	if item.SubReportID != 21 ||
		item.EvidenceSnapshotID != 7 ||
		item.DiagnosisTaskID != 31 ||
		item.SessionID != "diagnosis-session-31" ||
		item.ChatSessionID != 51 ||
		item.ConclusionStatus != "available" ||
		item.ConclusionSource != "latest_assistant_turn" ||
		item.Confidence != "high" ||
		item.RequiresHumanReview == nil ||
		!*item.RequiresHumanReview ||
		item.ConfidenceTimelineCount != 1 ||
		item.EvidenceRequestCount != 1 ||
		item.MissingEvidenceRequestCount != 1 ||
		item.EvidenceCollectionSuggestionCount != 1 ||
		item.EvidenceCollectionResultCount != 1 ||
		item.SupplementalEvidenceCount != 0 ||
		item.NotificationTimelineCount != 1 {
		t.Fatalf("reviewed subreport = %+v", item)
	}
}

func TestAttachAIReviewRejectsMissingEvidenceActivity(t *testing.T) {
	_, err := attachAIReview(
		context.Background(),
		fakeReportReader{
			finalReports: map[domain.FinalReportID]domain.FinalReport{
				11: {ID: 11},
			},
			subReports: map[domain.FinalReportID][]domain.SubReport{
				11: {testSubReport(21, 7)},
			},
		},
		fakeDiagnosisReader{
			tasks: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				7: {testDiagnosisTask(31, 7)},
			},
			events: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				31: {
					eventFinalReady: {
						testEvent(101, 31, eventFinalReady, `{
							"kind":"diagnosis_room.final_conclusion_ready",
							"session_id":"diagnosis-session-31",
							"chat_session_id":51,
							"diagnosis_task_id":31,
							"final_conclusion":{
								"status":"available",
								"source":"latest_assistant_turn",
								"evidence_snapshot_id":7,
								"content":"Final diagnosis retained in the database.",
								"confidence":"high",
								"requires_human_review":true
							}
						}`),
					},
					eventTurnPersisted: {
						testEvent(102, 31, eventTurnPersisted, `{
							"kind":"diagnosis_room.turn_persisted",
							"diagnosis_task_id":31,
							"confidence":"high"
						}`),
					},
				},
			},
		},
		testSmokeProof(),
		time.Date(2026, 5, 29, 1, 4, 0, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("attachAIReview: want missing evidence activity error")
	}
	if !strings.Contains(err.Error(), "has no evidence guidance, collection result, or supplemental evidence") {
		t.Fatalf("error = %q, want evidence activity rejection", err.Error())
	}
}

func TestAttachAIReviewBuildsPendingEvidenceProof(t *testing.T) {
	reviewedAt := time.Date(2026, 5, 29, 1, 4, 0, 0, time.UTC)
	proof, err := attachAIReview(
		context.Background(),
		fakeReportReader{
			finalReports: map[domain.FinalReportID]domain.FinalReport{
				11: {ID: 11},
			},
			subReports: map[domain.FinalReportID][]domain.SubReport{
				11: {testSubReport(21, 7)},
			},
		},
		fakeDiagnosisReader{
			tasks: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				7: {testDiagnosisTaskWithStatus(31, 7, domain.DiagnosisStatusRunning)},
			},
			events: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				31: {
					eventTurnPersisted: {
						testEvent(102, 31, eventTurnPersisted, `{
							"kind":"diagnosis_room.turn_persisted",
							"diagnosis_task_id":31,
							"confidence":"medium",
							"requires_human_review":true,
							"consultation_insight":{
								"conclusion_status":"needs_evidence",
								"missing_evidence_requests":[{"label":"pod logs","detail":"Need recent pod logs.","priority":"high"}],
								"evidence_collection_suggestions":[{"tool":"prometheus","query":"up"}]
							}
						}`),
					},
					eventAssistantTurnNotification: {
						testEvent(104, 31, eventAssistantTurnNotification, `{
							"kind":"diagnosis_room.assistant_turn_notification_sent",
							"diagnosis_task_id":31,
							"provider_status":"accepted"
						}`),
					},
				},
			},
		},
		testSmokeProof(),
		reviewedAt,
	)
	if err != nil {
		t.Fatalf("attachAIReview: %v", err)
	}
	if proof.AIReview == nil ||
		proof.AIReview.Status != "pending_evidence" ||
		len(proof.AIReview.ReviewedSubReports) != 0 ||
		len(proof.AIReview.PendingSubReports) != 1 {
		t.Fatalf("AIReview = %+v", proof.AIReview)
	}
	pending := proof.AIReview.PendingSubReports[0]
	if pending.SubReportID != 21 ||
		pending.EvidenceSnapshotID != 7 ||
		!strings.Contains(pending.Reason, "has no available diagnosis conclusion") ||
		len(pending.TaskStates) != 1 ||
		pending.TaskStates[0].LatestTurnStatus != "needs_evidence" ||
		pending.TaskStates[0].EvidenceRequestCount != 0 ||
		pending.TaskStates[0].MissingEvidenceRequestCount != 1 ||
		pending.TaskStates[0].EvidenceCollectionSuggestionCount != 1 ||
		pending.TaskStates[0].EvidenceCollectionResultCount != 0 ||
		pending.TaskStates[0].NotificationTimelineCount != 1 {
		t.Fatalf("pending review = %+v", pending)
	}
}

func TestAttachAIReviewBuildsFailedPendingEvidenceProof(t *testing.T) {
	reviewedAt := time.Date(2026, 5, 29, 1, 4, 0, 0, time.UTC)
	proof, err := attachAIReview(
		context.Background(),
		fakeReportReader{
			finalReports: map[domain.FinalReportID]domain.FinalReport{
				11: {ID: 11},
			},
			subReports: map[domain.FinalReportID][]domain.SubReport{
				11: {testSubReport(21, 7)},
			},
		},
		fakeDiagnosisReader{
			tasks: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				7: {{
					ID:                 31,
					EvidenceSnapshotID: 7,
					Status:             domain.DiagnosisStatusFailed,
					FailureReason:      "Diagnosis turn failed before an assistant response; upstream LLM request timed out.",
				}},
			},
			events: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				31: {
					eventFailed: {
						testEvent(105, 31, eventFailed, `{
							"kind":"diagnosis_room.failed",
							"diagnosis_task_id":31,
							"status":"failed",
							"failure_reason":"Diagnosis turn failed before an assistant response; upstream LLM request timed out."
						}`),
					},
				},
			},
		},
		testSmokeProof(),
		reviewedAt,
	)
	if err != nil {
		t.Fatalf("attachAIReview: %v", err)
	}
	if proof.AIReview == nil ||
		proof.AIReview.Status != "pending_evidence" ||
		len(proof.AIReview.PendingSubReports) != 1 {
		t.Fatalf("AIReview = %+v", proof.AIReview)
	}
	state := proof.AIReview.PendingSubReports[0].TaskStates[0]
	if state.TaskStatus != "failed" ||
		state.FailureReason != "Diagnosis turn failed before an assistant response; upstream LLM request timed out." ||
		state.FailedEventCount != 1 ||
		state.LastEventKind != eventFailed {
		t.Fatalf("failed task state = %+v", state)
	}
}

func TestAttachAIReviewRejectsUnreadyPendingEvidenceProof(t *testing.T) {
	_, err := attachAIReview(
		context.Background(),
		fakeReportReader{
			finalReports: map[domain.FinalReportID]domain.FinalReport{
				11: {ID: 11},
			},
			subReports: map[domain.FinalReportID][]domain.SubReport{
				11: {testSubReport(21, 7)},
			},
		},
		fakeDiagnosisReader{
			tasks: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				7: {testDiagnosisTaskWithStatus(31, 7, domain.DiagnosisStatusRunning)},
			},
			events: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				31: {
					eventTurnPersisted: {
						testEvent(102, 31, eventTurnPersisted, `{
							"kind":"diagnosis_room.turn_persisted",
							"diagnosis_task_id":31,
							"confidence":"medium",
							"requires_human_review":true,
							"evidence_requests":[{"tool":"active_alerts","reason":"Check sibling alerts."}],
							"consultation_insight":{"conclusion_status":"needs_evidence"}
						}`),
					},
				},
			},
		},
		testSmokeProof(),
		time.Date(2026, 5, 29, 1, 4, 0, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("attachAIReview: want unready pending state error")
	}
	if !strings.Contains(err.Error(), "pending diagnosis state is not ready") {
		t.Fatalf("error = %q, want pending readiness rejection", err.Error())
	}
}

func TestAttachAIReviewRejectsUnlinkedSubReport(t *testing.T) {
	_, err := attachAIReview(
		context.Background(),
		fakeReportReader{
			finalReports: map[domain.FinalReportID]domain.FinalReport{
				11: {ID: 11},
			},
			subReports: map[domain.FinalReportID][]domain.SubReport{
				11: {testSubReport(22, 7)},
			},
		},
		fakeDiagnosisReader{},
		testSmokeProof(),
		time.Date(2026, 5, 29, 1, 4, 0, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("attachAIReview: want unlinked subreport error")
	}
	if !strings.Contains(err.Error(), "is not linked to final report") {
		t.Fatalf("error = %q, want unlinked subreport rejection", err.Error())
	}
}

func TestAuditCandidatesReportsReadyAndMissingReviewEvidence(t *testing.T) {
	checkedAt := time.Date(2026, 5, 29, 1, 4, 0, 0, time.UTC)
	audit, err := auditCandidates(
		context.Background(),
		fakeReportReader{
			finalReports: map[domain.FinalReportID]domain.FinalReport{
				11: {ID: 11},
				12: {ID: 12},
			},
			finalReportOrder: []domain.FinalReportID{12, 11},
			subReports: map[domain.FinalReportID][]domain.SubReport{
				11: {testSubReport(21, 7)},
				12: {testSubReport(22, 8)},
			},
		},
		fakeDiagnosisReader{
			tasks: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				7: {testDiagnosisTask(31, 7)},
			},
			events: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				31: {
					eventFinalReady: {
						testEvent(101, 31, eventFinalReady, `{
							"kind":"diagnosis_room.final_conclusion_ready",
							"session_id":"diagnosis-session-31",
							"chat_session_id":51,
							"diagnosis_task_id":31,
							"final_conclusion":{
								"status":"available",
								"source":"latest_assistant_turn",
								"evidence_snapshot_id":7,
								"content":"Final diagnosis retained in the database.",
								"confidence":"high",
								"requires_human_review":true
							}
						}`),
					},
					eventTurnPersisted: {
						testEvent(102, 31, eventTurnPersisted, `{
							"kind":"diagnosis_room.turn_persisted",
							"diagnosis_task_id":31,
							"confidence":"medium",
							"evidence_requests":[{"tool":"active_alerts","reason":"Check sibling alerts."}]
						}`),
					},
				},
			},
		},
		20,
		checkedAt,
	)
	if err != nil {
		t.Fatalf("auditCandidates: %v", err)
	}
	if audit.CheckedAt != checkedAt.Format(time.RFC3339Nano) || audit.Limit != 20 || len(audit.Candidates) != 2 {
		t.Fatalf("audit = %+v", audit)
	}
	if audit.Candidates[0].FinalReportID != 12 ||
		audit.Candidates[0].AIReviewReady ||
		len(audit.Candidates[0].MissingReviewedEvidence) != 1 ||
		!strings.Contains(audit.Candidates[0].MissingReviewedEvidence[0].Reason, "has no diagnosis tasks") {
		t.Fatalf("missing candidate = %+v", audit.Candidates[0])
	}
	if audit.Candidates[1].FinalReportID != 11 ||
		!audit.Candidates[1].AIReviewReady ||
		audit.Candidates[1].ReviewedSubReportCount != 1 ||
		len(audit.Candidates[1].MissingReviewedEvidence) != 0 {
		t.Fatalf("ready candidate = %+v", audit.Candidates[1])
	}
}

func TestAuditCandidatesIncludesTaskStateForMissingFinalConclusion(t *testing.T) {
	requiresReview := true
	checkedAt := time.Date(2026, 5, 29, 1, 4, 0, 0, time.UTC)
	audit, err := auditCandidates(
		context.Background(),
		fakeReportReader{
			finalReports: map[domain.FinalReportID]domain.FinalReport{
				13: {ID: 13},
			},
			finalReportOrder: []domain.FinalReportID{13},
			subReports: map[domain.FinalReportID][]domain.SubReport{
				13: {testSubReport(23, 9)},
			},
		},
		fakeDiagnosisReader{
			tasks: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				9: {testDiagnosisTaskWithStatus(32, 9, domain.DiagnosisStatusRunning)},
			},
			events: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				32: {
					eventTurnPersisted: {
						testEvent(105, 32, eventTurnPersisted, `{
							"kind":"diagnosis_room.turn_persisted",
							"diagnosis_task_id":32,
							"confidence":"low",
							"requires_human_review":true,
							"evidence_requests":[{"tool":"active_alerts","reason":"Check sibling alerts."}],
							"consultation_insight":{
								"conclusion_status":"needs_evidence",
								"missing_evidence_requests":[{"label":"logs","detail":"Need app logs.","priority":"high"}]
							}
						}`),
					},
					eventEvidenceCollected: {
						testEvent(106, 32, eventEvidenceCollected, `{
							"kind":"diagnosis_room.evidence_collected",
							"diagnosis_task_id":32,
							"evidence_collection_results":[{"tool":"active_alerts","status":"collected"}]
						}`),
					},
					eventAssistantTurnNotification: {
						testEvent(107, 32, eventAssistantTurnNotification, `{
							"kind":"diagnosis_room.assistant_turn_notification_sent",
							"diagnosis_task_id":32,
							"provider_status":"delivered"
						}`),
					},
				},
			},
		},
		1,
		checkedAt,
	)
	if err != nil {
		t.Fatalf("auditCandidates: %v", err)
	}
	if len(audit.Candidates) != 1 {
		t.Fatalf("candidates = %+v", audit.Candidates)
	}
	missing := audit.Candidates[0].MissingReviewedEvidence
	if len(missing) != 1 || len(missing[0].TaskStates) != 1 {
		t.Fatalf("missing evidence = %+v", missing)
	}
	state := missing[0].TaskStates[0]
	if state.DiagnosisTaskID != 32 ||
		state.TaskStatus != "running" ||
		state.LatestTurnStatus != "needs_evidence" ||
		state.LatestConfidence != "low" ||
		state.LatestRequiresHumanReview == nil ||
		*state.LatestRequiresHumanReview != requiresReview ||
		state.ConfidenceTimelineCount != 1 ||
		state.EvidenceRequestCount != 1 ||
		state.MissingEvidenceRequestCount != 1 ||
		state.EvidenceCollectionSuggestionCount != 0 ||
		state.EvidenceCollectionResultCount != 1 ||
		state.NotificationTimelineCount != 1 ||
		state.FinalReadyEventCount != 0 ||
		state.ClosedEventCount != 0 ||
		state.LastEventKind != eventTurnPersisted {
		t.Fatalf("task state = %+v", state)
	}
}

func TestParseArgsDefaultsOutputAndDatabaseURL(t *testing.T) {
	cfg, err := parseArgs([]string{"--input", "proof.json"}, func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://example"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cfg.input != "proof.json" || cfg.output != "proof.json" || cfg.databaseURL != "postgres://example" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestParseArgsListCandidates(t *testing.T) {
	cfg, err := parseArgs([]string{"--list-candidates", "--limit", "3"}, func(key string) string {
		switch key {
		case "DATABASE_URL":
			return "postgres://example"
		case "REPORT_LIVE_SMOKE_AI_REVIEW_WAIT_TIMEOUT":
			return "2m"
		case "REPORT_LIVE_SMOKE_AI_REVIEW_POLL_INTERVAL":
			return "3s"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if !cfg.listCandidates || cfg.limit != 3 || cfg.input != "" || cfg.output != "" {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.waitTimeout != 0 || cfg.pollInterval != 0 {
		t.Fatalf("list candidate wait config = timeout %s interval %s, want zero", cfg.waitTimeout, cfg.pollInterval)
	}
}

func TestParseArgsWaitOptions(t *testing.T) {
	cfg, err := parseArgs([]string{"--input", "proof.json", "--wait-timeout", "2m", "--poll-interval", "3s"}, func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://example"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cfg.waitTimeout != 2*time.Minute || cfg.pollInterval != 3*time.Second {
		t.Fatalf("wait config = timeout %s interval %s", cfg.waitTimeout, cfg.pollInterval)
	}
}

func TestParseArgsRejectsPollIntervalWithoutWait(t *testing.T) {
	_, err := parseArgs([]string{"--input", "proof.json", "--poll-interval", "3s"}, func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://example"
		}
		return ""
	})
	if err == nil {
		t.Fatal("parseArgs: want poll without wait error")
	}
	if !strings.Contains(err.Error(), "requires --wait-timeout") {
		t.Fatalf("error = %q, want wait-timeout requirement", err.Error())
	}
}

func TestParseArgsRejectsListCandidatesWithInput(t *testing.T) {
	_, err := parseArgs([]string{"--list-candidates", "--input", "proof.json"}, func(string) string {
		return "postgres://example"
	})
	if err == nil {
		t.Fatal("parseArgs: want list/input conflict")
	}
	if !strings.Contains(err.Error(), "must be omitted") {
		t.Fatalf("error = %q, want omitted input/output error", err.Error())
	}
}

func TestAttachAIReviewWithRetryRetriesUntilSuccess(t *testing.T) {
	want := testSmokeProof()
	want.AIReview = &aiReviewProof{Status: "complete"}
	attempts := 0
	got, err := attachAIReviewWithRetry(
		context.Background(),
		100*time.Millisecond,
		time.Millisecond,
		func(context.Context, time.Time) (smokeOutput, error) {
			attempts++
			if attempts < 3 {
				return smokeOutput{}, errors.New("not ready")
			}
			return want, nil
		},
	)
	if err != nil {
		t.Fatalf("attachAIReviewWithRetry: %v", err)
	}
	if attempts != 3 || got.AIReview == nil {
		t.Fatalf("attempts=%d output=%+v", attempts, got)
	}
}

func TestAttachAIReviewWithRetryWithoutWaitDoesNotRetry(t *testing.T) {
	attempts := 0
	_, err := attachAIReviewWithRetry(
		context.Background(),
		0,
		0,
		func(context.Context, time.Time) (smokeOutput, error) {
			attempts++
			return smokeOutput{}, errors.New("not ready")
		},
	)
	if err == nil {
		t.Fatal("attachAIReviewWithRetry: want error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestWriteProofFileReplacesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proof.json")
	if err := os.WriteFile(path, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatalf("write old proof: %v", err)
	}
	proof := testSmokeProof()
	proof.AIReview = &aiReviewProof{
		Status:        "complete",
		ReviewedAt:    "2026-05-29T01:04:00Z",
		FinalReportID: 11,
	}
	if err := writeProofFile(path, proof); err != nil {
		t.Fatalf("writeProofFile: %v", err)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- test reads the temp proof path it created.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	if !strings.Contains(string(raw), `"ai_review"`) || strings.Contains(string(raw), `"old"`) {
		t.Fatalf("proof content = %s", string(raw))
	}
}

type fakeReportReader struct {
	finalReports     map[domain.FinalReportID]domain.FinalReport
	finalReportOrder []domain.FinalReportID
	subReports       map[domain.FinalReportID][]domain.SubReport
}

func (r fakeReportReader) FindFinalReportByID(_ context.Context, id domain.FinalReportID) (domain.FinalReport, error) {
	report, ok := r.finalReports[id]
	if !ok {
		return domain.FinalReport{}, errors.New("final report not found")
	}
	return report, nil
}

func (r fakeReportReader) ListSubReportsForFinalReport(_ context.Context, finalReportID domain.FinalReportID, _ int) ([]domain.SubReport, error) {
	return append([]domain.SubReport(nil), r.subReports[finalReportID]...), nil
}

func (r fakeReportReader) ListFinalReports(_ context.Context, limit int) ([]domain.FinalReport, error) {
	order := append([]domain.FinalReportID(nil), r.finalReportOrder...)
	if len(order) == 0 {
		for id := range r.finalReports {
			order = append(order, id)
		}
	}
	if limit < len(order) {
		order = order[:limit]
	}
	out := make([]domain.FinalReport, 0, len(order))
	for _, id := range order {
		out = append(out, r.finalReports[id])
	}
	return out, nil
}

type fakeDiagnosisReader struct {
	tasks  map[domain.EvidenceSnapshotID][]domain.DiagnosisTask
	events map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent
}

func (r fakeDiagnosisReader) ListTasksByEvidenceSnapshot(_ context.Context, snapshotID domain.EvidenceSnapshotID, _ int) ([]domain.DiagnosisTask, error) {
	return append([]domain.DiagnosisTask(nil), r.tasks[snapshotID]...), nil
}

func (r fakeDiagnosisReader) ListEventsByTaskAndKind(_ context.Context, taskID domain.DiagnosisTaskID, kind string, _ int) ([]domain.DiagnosisTaskEvent, error) {
	if r.events == nil || r.events[taskID] == nil {
		return nil, nil
	}
	return append([]domain.DiagnosisTaskEvent(nil), r.events[taskID][kind]...), nil
}

func testSmokeProof() smokeOutput {
	return smokeOutput{
		CheckedAt:  "2026-05-29T01:02:03Z",
		Started:    true,
		WorkflowID: "report-batch-1",
		RunID:      "run-1",
		Waited:     true,
		WorkflowResult: &workflowResult{
			SubReportIDs:               []int64{21},
			FinalReportID:              11,
			NotificationIdempotencyKey: "final_report:11/notification",
			NotificationStatus:         "accepted",
		},
		Snapshots: []snapshotRef{{
			ID:         7,
			GroupIndex: 0,
			EventCount: 1,
		}},
	}
}

func testSubReport(id domain.SubReportID, snapshotID domain.EvidenceSnapshotID) domain.SubReport {
	return domain.SubReport{
		ID:                 id,
		EvidenceSnapshotID: snapshotID,
	}
}

func testDiagnosisTask(id domain.DiagnosisTaskID, snapshotID domain.EvidenceSnapshotID) domain.DiagnosisTask {
	return testDiagnosisTaskWithStatus(id, snapshotID, "")
}

func testDiagnosisTaskWithStatus(id domain.DiagnosisTaskID, snapshotID domain.EvidenceSnapshotID, status domain.DiagnosisStatus) domain.DiagnosisTask {
	return domain.DiagnosisTask{
		ID:                 id,
		EvidenceSnapshotID: snapshotID,
		Status:             status,
	}
}

func testEvent(id domain.DiagnosisTaskEventID, taskID domain.DiagnosisTaskID, kind, payload string) domain.DiagnosisTaskEvent {
	occurredAt := time.Date(2026, 5, 29, 1, 3, int(id)%60, 0, time.UTC)
	return domain.DiagnosisTaskEvent{
		ID:         id,
		TaskID:     taskID,
		Kind:       kind,
		Payload:    []byte(payload),
		OccurredAt: occurredAt,
		RecordedAt: occurredAt.Add(time.Second),
	}
}
