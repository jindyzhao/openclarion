// Command egress_allowdeny_smoke provides helper processes for the manual M4
// Docker egress allow/deny smoke harness.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

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
	fs.Var(&allows, "allow", "allowed host[:port]; repeat for multiple targets")
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
