package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/providers/im/wecomcallback"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosiswecomcallback"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	rbacusecase "github.com/openclarion/openclarion/internal/usecases/rbac"
)

const maxDiagnosisWeComAppCallbackBodyBytes = 64 << 10

// DiagnosisWeComAppCallbackVerifier is the transport-facing Enterprise WeChat
// application message callback verifier.
type DiagnosisWeComAppCallbackVerifier interface {
	VerifyEcho(msgSignature, timestamp, nonce, echo string) (string, error)
	DecryptMessage(msgSignature, timestamp, nonce string, rawXML []byte) (wecomcallback.Message, error)
}

// DiagnosisWeComAppCallbackMessageHandler routes verified Enterprise WeChat
// application messages into diagnosis-room workflows.
type DiagnosisWeComAppCallbackMessageHandler interface {
	HandleMessage(ctx context.Context, req diagnosiswecomcallback.Request) (diagnosiswecomcallback.Result, error)
}

// WithDiagnosisWeComAppCallback enables Enterprise WeChat application message
// callback URL verification and encrypted callback acknowledgement.
func WithDiagnosisWeComAppCallback(verifier DiagnosisWeComAppCallbackVerifier) ServerOption {
	return func(s *Server) {
		s.weComAppCallback = verifier
	}
}

// WithDiagnosisWeComAppCallbackMessageHandler enables Enterprise WeChat app
// text messages to continue diagnosis-room conversations.
func WithDiagnosisWeComAppCallbackMessageHandler(handler DiagnosisWeComAppCallbackMessageHandler) ServerOption {
	return func(s *Server) {
		s.weComAppMessages = handler
	}
}

// WithDiagnosisWeComAppCallbackWorkflowRouter enables Enterprise WeChat app
// text messages to continue diagnosis-room conversations after local RBAC
// authorizes the sender for the referenced room.
func WithDiagnosisWeComAppCallbackWorkflowRouter(workflows ports.DiagnosisRoomWorkflowClient) ServerOption {
	return func(s *Server) {
		if workflows == nil {
			return
		}
		handler, err := diagnosiswecomcallback.NewService(
			workflows,
			diagnosiswecomcallback.WithRoomAuthorizer(diagnosisWeComAppCallbackRoomAuthorizer{server: s}),
			diagnosiswecomcallback.WithSenderResolver(diagnosisWeComAppCallbackSenderResolver{server: s}),
		)
		if err != nil {
			return
		}
		s.weComAppMessages = handler
	}
}

type diagnosisWeComAppCallbackRoomAuthorizer struct {
	server *Server
}

func (a diagnosisWeComAppCallbackRoomAuthorizer) AuthorizeDiagnosisRoomParticipation(
	ctx context.Context,
	subject string,
	sessionID string,
) (bool, error) {
	if a.server == nil {
		return false, fmt.Errorf("diagnosis wecom callback authorization: server is not configured: %w", domain.ErrInvariantViolation)
	}
	subject, err := sanitizeDiagnosisAuthSubject(subject)
	if err != nil {
		return false, fmt.Errorf("diagnosis wecom callback authorization: subject is invalid: %w", diagnosisauth.ErrUnauthenticated)
	}
	sessionID, err = normalizeRequiredID("session_id", sessionID)
	if err != nil {
		return false, err
	}
	if a.server.localRBACBootstrapAdminSubject(subject) {
		return true, nil
	}
	if a.server.rbacAuthorizer == nil {
		return false, fmt.Errorf("diagnosis wecom callback authorization: rbac authorizer is not configured: %w", domain.ErrInvariantViolation)
	}
	departmentKeys, err := a.server.localRBACDepartmentKeys(ctx, subject)
	if err != nil {
		return false, fmt.Errorf("diagnosis wecom callback authorization: resolve local rbac principal: %w", err)
	}
	principal := domain.RBACPrincipal{
		Subject:        subject,
		DepartmentKeys: departmentKeys,
	}
	ownerAllowed, err := a.server.diagnosisRoomOwnerAuthorizes(
		ctx,
		principal,
		domain.RBACPermissionDiagnosisRoomParticipate,
		domain.RBACScopeKindDiagnosisRoom,
		sessionID,
	)
	if err != nil {
		return false, fmt.Errorf("diagnosis wecom callback authorization: authorize owner: %w", err)
	}
	if ownerAllowed {
		return true, nil
	}
	decision, err := a.server.rbacAuthorizer.Authorize(ctx, rbacusecase.AuthorizeRequest{
		Principal:  principal,
		Permission: domain.RBACPermissionDiagnosisRoomParticipate,
		ScopeKind:  domain.RBACScopeKindDiagnosisRoom,
		ScopeKey:   sessionID,
	})
	if err != nil {
		return false, fmt.Errorf("diagnosis wecom callback authorization: authorize room participation: %w", err)
	}
	return decision.Allowed, nil
}

type diagnosisWeComAppCallbackSenderResolver struct {
	server *Server
}

func (r diagnosisWeComAppCallbackSenderResolver) ResolveWeComSenderSubject(
	ctx context.Context,
	wecomUserID string,
) (string, error) {
	if r.server == nil {
		return "", fmt.Errorf("diagnosis wecom callback sender resolver: server is not configured: %w", domain.ErrInvariantViolation)
	}
	wecomUserID = strings.TrimSpace(wecomUserID)
	if wecomUserID == "" {
		return "", fmt.Errorf("diagnosis wecom callback sender resolver: wecom user id is required: %w", domain.ErrInvariantViolation)
	}
	users, err := r.server.localRBACDirectoryUsersByExternalID(ctx, wecomUserID)
	if err != nil {
		return "", fmt.Errorf("diagnosis wecom callback sender resolver: resolve directory user: %w", err)
	}
	subject, ok, err := diagnosisWeComActiveDirectorySubject(users)
	if err != nil {
		return "", fmt.Errorf("diagnosis wecom callback sender resolver: resolve active directory subject: %w", err)
	}
	if !ok {
		return "", domain.ErrNotFound
	}
	return subject, nil
}

func diagnosisWeComActiveDirectorySubject(users []domain.DirectoryUser) (string, bool, error) {
	subject := ""
	for _, user := range users {
		if !user.Active {
			continue
		}
		current := strings.TrimSpace(user.Subject)
		if current == "" {
			continue
		}
		if subject == "" {
			subject = current
			continue
		}
		if subject != current {
			return "", false, fmt.Errorf("multiple active directory subjects share the same external id: %w", domain.ErrInvariantViolation)
		}
	}
	if subject == "" {
		return "", false, nil
	}
	return subject, true, nil
}

// VerifyDiagnosisWeComAppCallback implements api.ServerInterface.
func (s *Server) VerifyDiagnosisWeComAppCallback(w http.ResponseWriter, r *http.Request, params api.VerifyDiagnosisWeComAppCallbackParams) {
	if s.weComAppCallback == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "Enterprise WeChat app callback is not configured", nil)
		return
	}
	echo, err := s.weComAppCallback.VerifyEcho(
		params.MsgSignature,
		params.Timestamp,
		params.Nonce,
		params.Echostr,
	)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "Enterprise WeChat app callback verification failed", err)
		return
	}
	writePlainText(r.Context(), w, s.logger, http.StatusOK, echo)
}

// AcceptDiagnosisWeComAppCallback implements api.ServerInterface.
func (s *Server) AcceptDiagnosisWeComAppCallback(w http.ResponseWriter, r *http.Request, params api.AcceptDiagnosisWeComAppCallbackParams) {
	if s.weComAppCallback == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "Enterprise WeChat app callback is not configured", nil)
		return
	}
	defer r.Body.Close()
	rawBody, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDiagnosisWeComAppCallbackBodyBytes))
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "Enterprise WeChat app callback message rejected", err)
		return
	}
	message, err := s.weComAppCallback.DecryptMessage(
		params.MsgSignature,
		params.Timestamp,
		params.Nonce,
		rawBody,
	)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "Enterprise WeChat app callback message rejected", err)
		return
	}
	if s.weComAppMessages != nil {
		result, err := s.weComAppMessages.HandleMessage(r.Context(), diagnosiswecomcallback.Request{
			Message: message,
		})
		if err != nil {
			writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "Enterprise WeChat app callback message handling failed", err)
			return
		}
		if s.logger != nil {
			s.logger.InfoContext(
				r.Context(),
				"handled Enterprise WeChat app callback message",
				"status", result.Status,
				"session_id", result.SessionID,
				"message_id", result.MessageID,
			)
		}
	}
	writePlainText(r.Context(), w, s.logger, http.StatusOK, "success")
}

func writePlainText(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil && logger != nil {
		logger.ErrorContext(ctx, "write response", "error", err)
	}
}
