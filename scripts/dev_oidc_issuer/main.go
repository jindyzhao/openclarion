// Package main serves a local-only OIDC issuer for manual diagnosis-room smokes.
package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultListenAddr = "127.0.0.1:18080"
	defaultClientID   = "openclarion-web"
	defaultKeyID      = "openclarion-dev-oidc"
	defaultSubject    = "operator-1"
	defaultRoles      = "owner"
	defaultTokenTTL   = 30 * time.Minute
	maxTokenTTL       = 2 * time.Hour
)

type config struct {
	ListenAddr       string
	Issuer           string
	ClientID         string
	KeyID            string
	DefaultSubject   string
	DefaultRoles     []string
	DefaultTTL       time.Duration
	AllowNonLoopback bool
}

type devIssuer struct {
	issuer         string
	clientID       string
	keyID          string
	defaultSubject string
	defaultRoles   []string
	defaultTTL     time.Duration
	privateKey     *rsa.PrivateKey
	now            func() time.Time
}

type discoveryResponse struct {
	Issuer                           string   `json:"issuer"`
	JWKSURI                          string   `json:"jwks_uri"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type tokenResponse struct {
	TokenType           string   `json:"token_type"`
	IDToken             string   `json:"id_token"`
	AuthorizationHeader string   `json:"authorization_header"`
	Issuer              string   `json:"issuer"`
	ClientID            string   `json:"client_id"`
	Subject             string   `json:"subject"`
	Roles               []string `json:"roles"`
	ExpiresAt           string   `json:"expires_at"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "[dev-oidc-issuer] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	if err := validateLoopbackAddr(cfg.ListenAddr, cfg.AllowNonLoopback); err != nil {
		return err
	}
	listenerConfig := net.ListenConfig{}
	listener, err := listenerConfig.Listen(context.Background(), "tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()

	if strings.TrimSpace(cfg.Issuer) == "" {
		cfg.Issuer = "http://" + listener.Addr().String()
	}
	issuerURL, err := normalizeIssuer(cfg.Issuer, cfg.AllowNonLoopback)
	if err != nil {
		return err
	}
	cfg.Issuer = issuerURL

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate RSA key: %w", err)
	}
	issuer, err := newDevIssuer(cfg, privateKey)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "[dev-oidc-issuer] issuer: %s\n", issuer.issuer)
	fmt.Fprintf(stdout, "[dev-oidc-issuer] set OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL=%s\n", issuer.issuer)
	fmt.Fprintf(stdout, "[dev-oidc-issuer] set OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID=%s\n", issuer.clientID)
	fmt.Fprintf(stdout, "[dev-oidc-issuer] fetch token JSON: curl -fsS %s/token?subject=%s\n", issuer.endpoint("/token"), url.QueryEscape(issuer.defaultSubject))

	server := &http.Server{
		Handler:           issuer.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	var rawRoles string
	cfg := config{}
	fs := flag.NewFlagSet("dev-oidc-issuer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.ListenAddr, "listen", defaultListenAddr, "host:port to bind")
	fs.StringVar(&cfg.Issuer, "issuer", "", "issuer URL advertised in discovery; defaults to http://<listen-addr>")
	fs.StringVar(&cfg.ClientID, "client-id", defaultClientID, "OIDC audience/client ID")
	fs.StringVar(&cfg.KeyID, "kid", defaultKeyID, "JWKS key ID")
	fs.StringVar(&cfg.DefaultSubject, "subject", defaultSubject, "default token subject")
	fs.StringVar(&rawRoles, "roles", defaultRoles, "default comma-separated roles")
	fs.DurationVar(&cfg.DefaultTTL, "ttl", defaultTokenTTL, "default token lifetime")
	fs.BoolVar(&cfg.AllowNonLoopback, "allow-non-loopback", false, "allow non-loopback listen or issuer hosts")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	roles, err := parseCSV(rawRoles, "roles")
	if err != nil {
		return config{}, err
	}
	cfg.DefaultRoles = roles
	return cfg, nil
}

func newDevIssuer(cfg config, privateKey *rsa.PrivateKey) (*devIssuer, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("private key is required")
	}
	issuer, err := normalizeIssuer(cfg.Issuer, cfg.AllowNonLoopback)
	if err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("client id must be non-empty")
	}
	keyID := strings.TrimSpace(cfg.KeyID)
	if keyID == "" {
		return nil, fmt.Errorf("kid must be non-empty")
	}
	subject := strings.TrimSpace(cfg.DefaultSubject)
	if err := validateSubject(subject); err != nil {
		return nil, err
	}
	if err := validateTTL(cfg.DefaultTTL); err != nil {
		return nil, err
	}
	if len(cfg.DefaultRoles) == 0 {
		return nil, fmt.Errorf("roles must be non-empty")
	}
	return &devIssuer{
		issuer:         issuer,
		clientID:       clientID,
		keyID:          keyID,
		defaultSubject: subject,
		defaultRoles:   append([]string(nil), cfg.DefaultRoles...),
		defaultTTL:     cfg.DefaultTTL,
		privateKey:     privateKey,
		now:            time.Now,
	}, nil
}

func (i *devIssuer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", i.handleDiscovery)
	mux.HandleFunc("GET /keys", i.handleKeys)
	mux.HandleFunc("GET /token", i.handleToken)
	mux.HandleFunc("POST /token", i.handleToken)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func (i *devIssuer) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, discoveryResponse{
		Issuer:                           i.issuer,
		JWKSURI:                          i.endpoint("/keys"),
		AuthorizationEndpoint:            i.endpoint("/authorize"),
		TokenEndpoint:                    i.endpoint("/token"),
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ResponseTypesSupported:           []string{"id_token"},
		SubjectTypesSupported:            []string{"public"},
		ClaimsSupported:                  []string{"iss", "sub", "aud", "iat", "exp", "roles"},
	})
}

func (i *devIssuer) handleKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	pub := i.privateKey.PublicKey
	writeJSON(w, http.StatusOK, jwksResponse{Keys: []jwk{{
		Kty: "RSA",
		Use: "sig",
		Kid: i.keyID,
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

func (i *devIssuer) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	query := r.URL.Query()
	subject := strings.TrimSpace(query.Get("subject"))
	if subject == "" {
		subject = i.defaultSubject
	}
	if err := validateSubject(subject); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	audience := strings.TrimSpace(query.Get("audience"))
	if audience == "" {
		audience = i.clientID
	}
	if audience == "" {
		writeError(w, http.StatusBadRequest, "audience must be non-empty")
		return
	}
	roles := append([]string(nil), i.defaultRoles...)
	if _, ok := query["roles"]; ok {
		var err error
		roles, err = parseCSV(query.Get("roles"), "roles")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	ttl := i.defaultTTL
	if rawTTL := strings.TrimSpace(query.Get("ttl")); rawTTL != "" {
		parsed, err := time.ParseDuration(rawTTL)
		if err != nil {
			writeError(w, http.StatusBadRequest, "ttl must be a Go duration")
			return
		}
		ttl = parsed
	}
	if err := validateTTL(ttl); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	token, expiresAt, err := i.signIDToken(subject, audience, roles, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, tokenResponse{
		TokenType:           "Bearer",
		IDToken:             token,
		AuthorizationHeader: "Bearer " + token,
		Issuer:              i.issuer,
		ClientID:            audience,
		Subject:             subject,
		Roles:               roles,
		ExpiresAt:           expiresAt.Format(time.RFC3339),
	})
}

func (i *devIssuer) signIDToken(subject, audience string, roles []string, ttl time.Duration) (string, time.Time, error) {
	now := i.now().UTC().Truncate(time.Second)
	expiresAt := now.Add(ttl)
	header, err := jsonSegment(map[string]string{
		"alg": "RS256",
		"kid": i.keyID,
		"typ": "JWT",
	})
	if err != nil {
		return "", time.Time{}, err
	}
	payload, err := jsonSegment(map[string]any{
		"iss":   i.issuer,
		"sub":   subject,
		"aud":   audience,
		"iat":   now.Unix(),
		"exp":   expiresAt.Unix(),
		"roles": roles,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	signingInput := header + "." + payload
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, i.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), expiresAt, nil
}

func jsonSegment(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func (i *devIssuer) endpoint(path string) string {
	return strings.TrimRight(i.issuer, "/") + path
}

func parseCSV(raw, label string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("%s must not contain empty values", label)
		}
		out = append(out, part)
	}
	return out, nil
}

func validateSubject(subject string) error {
	if subject == "" {
		return fmt.Errorf("subject must be non-empty")
	}
	if len(subject) > 128 {
		return fmt.Errorf("subject must be at most 128 bytes")
	}
	if strings.ContainsAny(subject, "\r\n\t ") {
		return fmt.Errorf("subject must be a single token")
	}
	return nil
}

func validateTTL(ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("ttl must be positive")
	}
	if ttl > maxTokenTTL {
		return fmt.Errorf("ttl must not exceed %s", maxTokenTTL)
	}
	return nil
}

func normalizeIssuer(raw string, allowNonLoopback bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("issuer must be non-empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse issuer: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("issuer must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("issuer must be absolute")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("issuer must not include userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("issuer must not include path, query, or fragment")
	}
	if err := validateLoopbackHost(parsed.Hostname(), allowNonLoopback, "issuer host"); err != nil {
		return "", err
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func validateLoopbackAddr(addr string, allowNonLoopback bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen address must be host:port: %w", err)
	}
	return validateLoopbackHost(host, allowNonLoopback, "listen address")
}

func validateLoopbackHost(host string, allowNonLoopback bool, label string) error {
	host = strings.Trim(host, "[]")
	if host == "" {
		if allowNonLoopback {
			return nil
		}
		return fmt.Errorf("%s must be loopback unless --allow-non-loopback is set", label)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	if allowNonLoopback {
		return nil
	}
	return fmt.Errorf("%s must be loopback unless --allow-non-loopback is set", label)
}

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST")
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
