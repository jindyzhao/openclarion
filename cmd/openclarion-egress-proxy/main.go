// Command openclarion-egress-proxy runs the local sandbox egress boundary.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/openclarion/openclarion/internal/egressproxy"
	"golang.org/x/sync/errgroup"
)

const (
	allowedTargetsEnv = "OPENCLARION_EGRESS_PROXY_ALLOWED"
	listenAddrEnv     = "OPENCLARION_EGRESS_PROXY_LISTEN_ADDR"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("egress proxy exited", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "healthcheck" {
		if len(args) != 2 {
			return fmt.Errorf("usage: openclarion-egress-proxy healthcheck URL")
		}
		return healthcheck(args[1])
	}
	if len(args) > 0 && args[0] != "serve" {
		return fmt.Errorf("unknown command %q (expected serve or healthcheck)", args[0])
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: openclarion-egress-proxy [serve]")
	}

	allowed := splitCSV(os.Getenv(allowedTargetsEnv))
	handler, err := egressproxy.NewHandler(egressproxy.Config{AllowedTargets: allowed})
	if err != nil {
		return err
	}
	defer handler.Close()
	listenAddr := strings.TrimSpace(os.Getenv(listenAddrEnv))
	if listenAddr == "" {
		listenAddr = ":18080"
	}
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(server.ListenAndServe)
	group.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	})
	slog.Info("sandbox egress proxy listening", "addr", listenAddr, "allowed_target_count", len(allowed))
	err = group.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func healthcheck(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" || parsed.Path != "/healthz" {
		return fmt.Errorf("healthcheck URL must be an absolute http loopback /healthz URL without credentials, query, or fragment")
	}
	host := net.ParseIP(parsed.Hostname())
	if host == nil || !host.IsLoopback() {
		return fmt.Errorf("healthcheck URL host must be a literal loopback IP")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   2 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// #nosec G704 -- the parsed URL is restricted above to a literal loopback
	// address and the fixed /healthz path.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req) // #nosec G704 -- req is restricted to loopback /healthz above.
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	return nil
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
