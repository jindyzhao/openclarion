package diagnosiswecomcallback

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/providers/im/wecomcallback"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestHandleMessageSubmitsTextWithSessionURL(t *testing.T) {
	workflows := &recordingWorkflowClient{}
	authorizer := &recordingRoomAuthorizer{allowed: true}
	service := newAuthorizedService(t, workflows, authorizer)

	result, err := service.HandleMessage(context.Background(), Request{
		Message: wecomcallback.Message{
			FromUserName: "operator-1",
			CreateTime:   1700000000,
			MsgID:        "wecom-msg-1",
			MsgType:      "text",
			Content:      "Please re-check https://console.example.test/diagnosis-room?session_id=diagnosis-session-abc123&auth_mode=session",
		},
	})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if result.Status != StatusSubmitted ||
		result.SessionID != "diagnosis-session-abc123" ||
		result.ActorSubject != "operator-1" ||
		!strings.HasPrefix(result.MessageID, "wecom-app:") {
		t.Fatalf("result = %+v", result)
	}
	got, called := workflows.submitSnapshot()
	if called != 1 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 1", called)
	}
	if got.SessionID != "diagnosis-session-abc123" ||
		got.ActorSubject != "operator-1" ||
		got.Message != "Please re-check https://console.example.test/diagnosis-room?session_id=diagnosis-session-abc123&auth_mode=session" ||
		got.MessageID != result.MessageID {
		t.Fatalf("submit request = %+v", got)
	}
	if authorizer.called != 1 ||
		authorizer.subject != "operator-1" ||
		authorizer.sessionID != "diagnosis-session-abc123" {
		t.Fatalf("authorizer subject=%q session=%q called=%d", authorizer.subject, authorizer.sessionID, authorizer.called)
	}
}

func TestHandleMessageExtractsSessionFromPlainTextAndEventKey(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		event   string
		want    string
	}{
		{
			name:    "plain session token",
			content: "diagnosis-session-auto-p3-s247 please collect the JVM heap evidence",
			want:    "diagnosis-session-auto-p3-s247",
		},
		{
			name:    "query string",
			content: "session_id=diagnosis-session-42&auth_mode=session",
			want:    "diagnosis-session-42",
		},
		{
			name:    "event key fallback",
			content: "collect the missing evidence",
			event:   "room:diagnosis-session-event-1",
			want:    "diagnosis-session-event-1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflows := &recordingWorkflowClient{}
			service := newAuthorizedService(t, workflows, &recordingRoomAuthorizer{allowed: true})

			result, err := service.HandleMessage(context.Background(), Request{
				Message: wecomcallback.Message{
					FromUserName: "operator-1",
					MsgType:      "text",
					Content:      tc.content,
					EventKey:     tc.event,
				},
			})
			if err != nil {
				t.Fatalf("HandleMessage: %v", err)
			}
			if result.Status != StatusSubmitted || result.SessionID != tc.want {
				t.Fatalf("result = %+v, want submitted session %q", result, tc.want)
			}
		})
	}
}

func TestHandleMessageSkipsUnsupportedMessages(t *testing.T) {
	service, err := NewService(&recordingWorkflowClient{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	tests := []struct {
		name    string
		message wecomcallback.Message
		want    Status
	}{
		{
			name: "non text",
			message: wecomcallback.Message{
				FromUserName: "operator-1",
				MsgType:      "event",
				Event:        "click",
			},
			want: StatusSkippedNonText,
		},
		{
			name: "empty text",
			message: wecomcallback.Message{
				FromUserName: "operator-1",
				MsgType:      "text",
				Content:      " ",
			},
			want: StatusSkippedEmptyText,
		},
		{
			name: "missing session",
			message: wecomcallback.Message{
				FromUserName: "operator-1",
				MsgType:      "text",
				Content:      "What is the current confidence?",
			},
			want: StatusSkippedNoSession,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := service.HandleMessage(context.Background(), Request{Message: tc.message})
			if err != nil {
				t.Fatalf("HandleMessage: %v", err)
			}
			if result.Status != tc.want {
				t.Fatalf("status = %q, want %q", result.Status, tc.want)
			}
		})
	}
}

func TestHandleMessageSkipsUnauthorizedSender(t *testing.T) {
	workflows := &recordingWorkflowClient{}
	service := newAuthorizedService(t, workflows, &recordingRoomAuthorizer{allowed: false})

	result, err := service.HandleMessage(context.Background(), Request{
		Message: wecomcallback.Message{
			FromUserName: "operator-1",
			MsgType:      "text",
			Content:      "diagnosis-session-1 please continue",
		},
	})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if result.Status != StatusSkippedUnauthorized ||
		result.SessionID != "diagnosis-session-1" ||
		result.ActorSubject != "operator-1" {
		t.Fatalf("result = %+v, want skipped unauthorized", result)
	}
	if _, called := workflows.submitSnapshot(); called != 0 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 0", called)
	}
}

func TestHandleMessageResolvesWeComSenderBeforeAuthorization(t *testing.T) {
	workflows := &recordingWorkflowClient{}
	authorizer := &recordingRoomAuthorizer{allowed: true}
	service, err := NewService(
		workflows,
		WithRoomAuthorizer(authorizer),
		WithSenderResolver(recordingSenderResolver{
			subjectsByWeComUserID: map[string]string{
				"wecom-operator-1": "operator-1",
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := service.HandleMessage(context.Background(), Request{
		Message: wecomcallback.Message{
			FromUserName: "wecom-operator-1",
			MsgType:      "text",
			Content:      "diagnosis-session-1 please continue",
		},
	})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if result.Status != StatusSubmitted ||
		result.SessionID != "diagnosis-session-1" ||
		result.ActorSubject != "operator-1" {
		t.Fatalf("result = %+v, want resolved submitted actor", result)
	}
	if authorizer.called != 1 ||
		authorizer.subject != "operator-1" ||
		authorizer.sessionID != "diagnosis-session-1" {
		t.Fatalf("authorizer subject=%q session=%q called=%d", authorizer.subject, authorizer.sessionID, authorizer.called)
	}
	got, called := workflows.submitSnapshot()
	if called != 1 ||
		got.ActorSubject != "operator-1" ||
		got.SessionID != "diagnosis-session-1" {
		t.Fatalf("submit request = %+v called=%d, want resolved actor", got, called)
	}
}

func TestHandleMessageSkipsUnknownResolvedWeComSender(t *testing.T) {
	workflows := &recordingWorkflowClient{}
	authorizer := &recordingRoomAuthorizer{allowed: true}
	service, err := NewService(
		workflows,
		WithRoomAuthorizer(authorizer),
		WithSenderResolver(recordingSenderResolver{}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := service.HandleMessage(context.Background(), Request{
		Message: wecomcallback.Message{
			FromUserName: "unknown-wecom-user",
			MsgType:      "text",
			Content:      "diagnosis-session-1 please continue",
		},
	})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if result.Status != StatusSkippedUnauthorized ||
		result.SessionID != "diagnosis-session-1" ||
		result.ActorSubject != "unknown-wecom-user" {
		t.Fatalf("result = %+v, want skipped unknown sender", result)
	}
	if authorizer.called != 0 {
		t.Fatalf("authorizer calls = %d, want 0", authorizer.called)
	}
	if _, called := workflows.submitSnapshot(); called != 0 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 0", called)
	}
}

func TestHandleMessageRequiresRoomAuthorizerBeforeSubmitting(t *testing.T) {
	workflows := &recordingWorkflowClient{}
	service, err := NewService(workflows)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := service.HandleMessage(context.Background(), Request{
		Message: wecomcallback.Message{
			FromUserName: "operator-1",
			MsgType:      "text",
			Content:      "diagnosis-session-1 please continue",
		},
	})
	if err == nil {
		t.Fatal("HandleMessage error = nil, want missing authorizer error")
	}
	if result.Status != StatusSkippedUnauthorized {
		t.Fatalf("status = %q, want %q", result.Status, StatusSkippedUnauthorized)
	}
	if _, called := workflows.submitSnapshot(); called != 0 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 0", called)
	}
}

func TestHandleMessagePropagatesSubmitErrors(t *testing.T) {
	wantErr := errors.New("workflow unavailable")
	service := newAuthorizedService(t, &recordingWorkflowClient{submitErr: wantErr}, &recordingRoomAuthorizer{allowed: true})

	_, err := service.HandleMessage(context.Background(), Request{
		Message: wecomcallback.Message{
			FromUserName: "operator-1",
			MsgType:      "text",
			Content:      "diagnosis-session-1 please continue",
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("HandleMessage error = %v, want %v", err, wantErr)
	}
}

func TestExtractDiagnosisSessionID(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "url query",
			raw:  "https://console.example.test/diagnosis-room?session_id=diagnosis-session-abc&auth_mode=session",
			want: "diagnosis-session-abc",
		},
		{
			name: "plain query",
			raw:  "session_id=diagnosis-session-query&auth_mode=session",
			want: "diagnosis-session-query",
		},
		{
			name: "sentence punctuation",
			raw:  "Use diagnosis-session-auto-p3-s247.",
			want: "diagnosis-session-auto-p3-s247",
		},
		{
			name: "missing",
			raw:  "no room here",
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractDiagnosisSessionID(tc.raw); got != tc.want {
				t.Fatalf("ExtractDiagnosisSessionID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func newAuthorizedService(
	t *testing.T,
	workflows ports.DiagnosisRoomWorkflowClient,
	authorizer RoomAuthorizer,
) *Service {
	t.Helper()
	service, err := NewService(workflows, WithRoomAuthorizer(authorizer))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type recordingRoomAuthorizer struct {
	allowed   bool
	err       error
	called    int
	subject   string
	sessionID string
}

func (a *recordingRoomAuthorizer) AuthorizeDiagnosisRoomParticipation(_ context.Context, subject, sessionID string) (bool, error) {
	a.called++
	a.subject = subject
	a.sessionID = sessionID
	if a.err != nil {
		return false, a.err
	}
	return a.allowed, nil
}

type recordingSenderResolver struct {
	subjectsByWeComUserID map[string]string
	err                   error
}

func (r recordingSenderResolver) ResolveWeComSenderSubject(_ context.Context, wecomUserID string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	subject, ok := r.subjectsByWeComUserID[wecomUserID]
	if !ok {
		return "", domain.ErrNotFound
	}
	return subject, nil
}

type recordingWorkflowClient struct {
	submitReq ports.DiagnosisRoomSubmitTurnRequest
	submitErr error
	called    int
}

func (c *recordingWorkflowClient) SubmitDiagnosisTurn(_ context.Context, req ports.DiagnosisRoomSubmitTurnRequest) (ports.DiagnosisRoomSubmitTurnResult, error) {
	c.called++
	c.submitReq = req
	if c.submitErr != nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, c.submitErr
	}
	return ports.DiagnosisRoomSubmitTurnResult{SessionID: req.SessionID, MessageID: req.MessageID}, nil
}

func (c *recordingWorkflowClient) CollectDiagnosisEvidence(context.Context, ports.DiagnosisRoomCollectEvidenceRequest) (ports.DiagnosisRoomCollectEvidenceResult, error) {
	return ports.DiagnosisRoomCollectEvidenceResult{}, nil
}

func (c *recordingWorkflowClient) ConfirmDiagnosisConclusion(context.Context, ports.DiagnosisRoomConfirmConclusionRequest) (ports.DiagnosisRoomState, error) {
	return ports.DiagnosisRoomState{}, nil
}

func (c *recordingWorkflowClient) QueryDiagnosisRoom(context.Context, string) (ports.DiagnosisRoomState, error) {
	return ports.DiagnosisRoomState{}, nil
}

func (c *recordingWorkflowClient) submitSnapshot() (ports.DiagnosisRoomSubmitTurnRequest, int) {
	return c.submitReq, c.called
}
