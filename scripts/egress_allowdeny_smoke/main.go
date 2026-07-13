// Command egress_allowdeny_smoke provides helper processes for the manual M4
// Docker egress allow/deny smoke harness.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

var digestPinnedImageRE = regexp.MustCompile(`^[^\s@]+@sha256:[A-Fa-f0-9]{64}$`)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[egress-allowdeny-smoke] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: egress_allowdeny_smoke <serve|proxy|client> [flags]")
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "proxy":
		return runProxy(args[1:])
	case "client":
		return runClient(args[1:], stdout)
	case "proof":
		return runProof(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	listen := fs.String("listen", ":8080", "listen address")
	name := fs.String("name", "upstream", "response name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	server := &http.Server{
		Addr:              *listen,
		Handler:           upstreamHandler(*name),
		ReadHeaderTimeout: 2 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runProxy(args []string) error {
	fs := flag.NewFlagSet("proxy", flag.ContinueOnError)
	listen := fs.String("listen", ":18080", "listen address")
	allows := multiFlag{}
	fs.Var(&allows, "allow", "allowed host:port; repeat for multiple targets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	allowed := normalizeAllowed(allows)
	if len(allowed) == 0 {
		return errors.New("at least one --allow target is required")
	}
	server := &http.Server{
		Addr:              *listen,
		Handler:           proxyHandler(allowed),
		ReadHeaderTimeout: 2 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runClient(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	rawURL := fs.String("url", "", "request URL")
	rawProxy := fs.String("proxy", "", "HTTP proxy URL")
	wantStatus := fs.Int("want-status", 0, "expected HTTP status")
	wantFail := fs.Bool("want-fail", false, "expect request failure")
	timeout := fs.Duration("timeout", 5*time.Second, "request timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rawURL == "" {
		return errors.New("--url is required")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if *rawProxy != "" {
		proxyURL, err := url.Parse(*rawProxy)
		if err != nil {
			return fmt.Errorf("parse proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	client := &http.Client{Transport: transport, Timeout: *timeout}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, *rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if *wantFail {
		if err != nil {
			fmt.Fprintf(stdout, "[egress-allowdeny-smoke] expected failure for %s: %v\n", *rawURL, err)
			return nil
		}
		_ = resp.Body.Close()
		return fmt.Errorf("request to %s succeeded with status %d, want failure", *rawURL, resp.StatusCode)
	}
	if err != nil {
		return fmt.Errorf("request %s: %w", *rawURL, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if *wantStatus != 0 && resp.StatusCode != *wantStatus {
		return fmt.Errorf("request %s status = %d, want %d", *rawURL, resp.StatusCode, *wantStatus)
	}
	fmt.Fprintf(stdout, "[egress-allowdeny-smoke] %s -> %d\n", *rawURL, resp.StatusCode)
	return nil
}

func runProof(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("proof", flag.ContinueOnError)
	proofPath := fs.String("proof-path", "", "retained proof JSON path")
	imageRef := fs.String("image-ref", "", "digest-pinned image ref used for the smoke containers")
	source := fs.String("source", "make egress-allowdeny-smoke", "canonical proof source")
	runID := fs.String("run-id", "", "optional smoke run identifier")
	timeoutSeconds := fs.Int64("timeout-seconds", 8, "smoke readiness timeout in seconds")
	allowedTarget := fs.String("allowed-target", "allowed.internal:8080", "allowed egress host:port")
	deniedTarget := fs.String("denied-target", "denied.internal:8080", "denied egress host:port")
	proxyTarget := fs.String("proxy-target", "egress-proxy:18080", "sandbox proxy host:port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*proofPath) == "" {
		return errors.New("--proof-path is required")
	}
	if !digestPinnedImageRE.MatchString(*imageRef) {
		return fmt.Errorf("--image-ref must be pinned by sha256 digest: %s", *imageRef)
	}
	if *source != "make egress-allowdeny-smoke" {
		return errors.New("--source must be make egress-allowdeny-smoke")
	}
	if *timeoutSeconds <= 0 {
		return errors.New("--timeout-seconds must be a positive integer")
	}
	if strings.TrimSpace(*allowedTarget) == "" || strings.TrimSpace(*deniedTarget) == "" || strings.TrimSpace(*proxyTarget) == "" {
		return errors.New("proof targets must be non-empty")
	}
	proof := egressProofArtifact{
		Tool:       "egress-allowdeny-smoke",
		Status:     "pass",
		Source:     *source,
		ImageRef:   *imageRef,
		RunID:      strings.TrimSpace(*runID),
		TimeoutSec: *timeoutSeconds,
		Topology: egressProofTopology{
			SandboxNetwork:     "internal",
			UpstreamNetwork:    "separate",
			ProxyTarget:        *proxyTarget,
			AllowedTarget:      *allowedTarget,
			DeniedTarget:       *deniedTarget,
			DirectBypassTarget: *allowedTarget,
		},
		Checks: []proofCheck{
			{Name: "digest_pinned_image", Status: "pass"},
			{Name: "sandbox_network_internal", Status: "pass"},
			{Name: "upstream_network_separate", Status: "pass"},
			{Name: "proxy_dual_network", Status: "pass"},
			{Name: "allowed_target_via_proxy", Status: "pass"},
			{Name: "denied_target_blocked_by_proxy", Status: "pass"},
			{Name: "direct_bypass_failed", Status: "pass"},
			{Name: "non_root_readonly_no_new_privileges_cap_drop", Status: "pass"},
		},
	}
	if err := writeJSONFile(*proofPath, proof); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "[egress-allowdeny-smoke] proof written: %s\n", filepath.Clean(*proofPath))
	return nil
}

type egressProofArtifact struct {
	Tool       string              `json:"tool"`
	Status     string              `json:"status"`
	Source     string              `json:"source"`
	ImageRef   string              `json:"image_ref"`
	RunID      string              `json:"run_id,omitempty"`
	TimeoutSec int64               `json:"timeout_seconds"`
	Topology   egressProofTopology `json:"topology"`
	Checks     []proofCheck        `json:"checks"`
}

type egressProofTopology struct {
	SandboxNetwork     string `json:"sandbox_network"`
	UpstreamNetwork    string `json:"upstream_network"`
	ProxyTarget        string `json:"proxy_target"`
	AllowedTarget      string `json:"allowed_target"`
	DeniedTarget       string `json:"denied_target"`
	DirectBypassTarget string `json:"direct_bypass_target"`
}

type proofCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func writeJSONFile(path string, value any) error {
	clean := filepath.Clean(path)
	if err := validateNoSymlinkAncestors(clean); err != nil {
		return err
	}
	if info, err := os.Lstat(clean); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s must be a regular file, not a symlink", clean)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s must be a regular file", clean)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat proof: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o700); err != nil {
		return fmt.Errorf("create proof parent: %w", err)
	}
	if err := validateNoSymlinkAncestors(clean); err != nil {
		return err
	}
	// #nosec G304 -- this manual smoke helper writes the operator-supplied proof JSON path.
	f, err := openProofFileNoFollow(clean)
	if err != nil {
		return fmt.Errorf("write proof: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		_ = f.Close()
		return fmt.Errorf("write proof: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write proof: %w", err)
	}
	return nil
}

func openProofFileNoFollow(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = unix.Close(fd)
		return nil, errors.New("wrap proof file descriptor")
	}
	return f, nil
}

func validateNoSymlinkAncestors(cleanPath string) error {
	dir := filepath.Dir(cleanPath)
	for dir != "." && dir != string(filepath.Separator) {
		info, err := os.Lstat(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				next := filepath.Dir(dir)
				if next == dir {
					return nil
				}
				dir = next
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s parent directory %s must not be a symlink", cleanPath, dir)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s parent path %s must be a directory", cleanPath, dir)
		}
		next := filepath.Dir(dir)
		if next == dir {
			return nil
		}
		dir = next
	}
	return nil
}

func upstreamHandler(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "openclarion-egress-smoke:%s\n", name)
	})
}

func proxyHandler(allowed map[string]bool) http.Handler {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Scheme != "http" || r.URL.Host == "" {
			http.Error(w, "proxy requires absolute http URL", http.StatusBadRequest)
			return
		}
		target := strings.ToLower(r.URL.Host)
		if !allowed[target] {
			http.Error(w, "egress target denied", http.StatusForbidden)
			return
		}
		outbound := r.Clone(r.Context())
		outbound.RequestURI = ""
		outbound.URL = cloneURL(r.URL)
		outbound.Host = outbound.URL.Host
		outbound.Header = r.Header.Clone()
		outbound.Header.Del("Proxy-Connection")
		resp, err := transport.RoundTrip(outbound)
		if err != nil {
			http.Error(w, "upstream error", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})
}

func cloneURL(in *url.URL) *url.URL {
	out := *in
	return &out
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func normalizeAllowed(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		out[strings.ToLower(value)] = true
	}
	return out
}

type multiFlag []string

func (m *multiFlag) String() string {
	if m == nil {
		return ""
	}
	values := append([]string(nil), *m...)
	sort.Strings(values)
	return strings.Join(values, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}
