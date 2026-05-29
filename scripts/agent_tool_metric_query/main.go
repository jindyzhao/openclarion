// Command agent_tool_metric_query is a narrow Prometheus instant-query helper
// intended for sandbox runtime images. It reads configuration from flags/env,
// calls /api/v1/query, and writes one JSON object to stdout.
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
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	envPrometheusURL   = "OPENCLARION_TOOL_PROMETHEUS_URL"
	envBearerToken     = "OPENCLARION_TOOL_PROMETHEUS_BEARER_TOKEN" // #nosec G101 -- environment variable name, not a credential value.
	defaultHTTPTimeout = 10 * time.Second
	maxResponseBytes   = 4 * 1024 * 1024
)

type config struct {
	PrometheusURL string
	Query         string
	Time          string
	QueryTimeout  time.Duration
	Limit         int
	HTTPTimeout   time.Duration
	BearerToken   string
}

type prometheusEnvelope struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType,omitempty"`
	Error     string          `json:"error,omitempty"`
	Warnings  []string        `json:"warnings,omitempty"`
	Infos     []string        `json:"infos,omitempty"`
}

type queryData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

type output struct {
	Tool       string          `json:"tool"`
	Source     string          `json:"source"`
	Query      string          `json:"query"`
	Time       string          `json:"time,omitempty"`
	ResultType string          `json:"result_type"`
	Result     json.RawMessage `json:"result"`
	Warnings   []string        `json:"warnings,omitempty"`
	Infos      []string        `json:"infos,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Getenv, http.DefaultClient); err != nil {
		fmt.Fprintf(os.Stderr, "[agent-tool-metric-query] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, getenv func(string) string, client *http.Client) error {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTPTimeout)
	defer cancel()

	out, err := queryPrometheus(ctx, cfg, client)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	fs := flag.NewFlagSet("agent_tool_metric_query", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	promURL := fs.String("prometheus-url", strings.TrimSpace(getenv(envPrometheusURL)), "Prometheus base URL")
	query := fs.String("query", "", "PromQL instant query")
	queryTime := fs.String("time", "", "optional RFC3339 or unix timestamp")
	queryTimeout := fs.Duration("query-timeout", defaultHTTPTimeout, "Prometheus query timeout parameter")
	limit := fs.Int("limit", 100, "maximum number of returned series")
	httpTimeout := fs.Duration("http-timeout", defaultHTTPTimeout, "HTTP client timeout")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg := config{
		PrometheusURL: strings.TrimSpace(*promURL),
		Query:         strings.TrimSpace(*query),
		Time:          strings.TrimSpace(*queryTime),
		QueryTimeout:  *queryTimeout,
		Limit:         *limit,
		HTTPTimeout:   *httpTimeout,
		BearerToken:   strings.TrimSpace(getenv(envBearerToken)),
	}
	if cfg.PrometheusURL == "" {
		return config{}, fmt.Errorf("%s or --prometheus-url is required", envPrometheusURL)
	}
	if cfg.Query == "" {
		return config{}, errors.New("--query is required")
	}
	if cfg.QueryTimeout <= 0 {
		return config{}, errors.New("--query-timeout must be positive")
	}
	if cfg.HTTPTimeout <= 0 {
		return config{}, errors.New("--http-timeout must be positive")
	}
	if cfg.Limit <= 0 || cfg.Limit > 10000 {
		return config{}, errors.New("--limit must be between 1 and 10000")
	}
	if _, err := validatedBaseURL(cfg.PrometheusURL); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func queryPrometheus(ctx context.Context, cfg config, client *http.Client) (output, error) {
	if client == nil {
		client = http.DefaultClient
	}
	endpoint, err := queryEndpoint(cfg)
	if err != nil {
		return output{}, err
	}
	// #nosec G704 -- the helper intentionally calls the operator-configured
	// Prometheus endpoint; sandbox egress policy remains the network boundary.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return output{}, fmt.Errorf("build Prometheus query request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if cfg.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.BearerToken)
	}
	// #nosec G704 -- see request construction above; URL is validated and the
	// sandbox runtime must still pass Docker egress enforcement.
	resp, err := client.Do(req)
	if err != nil {
		return output{}, fmt.Errorf("query Prometheus: %w", err)
	}
	defer resp.Body.Close()
	body, err := readCapped(resp.Body, maxResponseBytes)
	if err != nil {
		return output{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return output{}, fmt.Errorf("Prometheus query status %d", resp.StatusCode)
	}
	return parsePrometheusOutput(cfg, body)
}

func queryEndpoint(cfg config) (string, error) {
	base, err := validatedBaseURL(cfg.PrometheusURL)
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v1/query"
	values := base.Query()
	values.Set("query", cfg.Query)
	values.Set("timeout", formatDurationSeconds(cfg.QueryTimeout))
	values.Set("limit", strconv.Itoa(cfg.Limit))
	if cfg.Time != "" {
		values.Set("time", cfg.Time)
	}
	base.RawQuery = values.Encode()
	return base.String(), nil
}

func validatedBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse Prometheus URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("Prometheus URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("Prometheus URL must include a host")
	}
	if parsed.User != nil {
		return nil, errors.New("Prometheus URL must not include userinfo")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func parsePrometheusOutput(cfg config, body []byte) (output, error) {
	var envelope prometheusEnvelope
	if err := strictjson.Unmarshal(body, &envelope); err != nil {
		return output{}, fmt.Errorf("decode Prometheus response: %w", err)
	}
	if envelope.Status != "success" {
		if envelope.Error != "" {
			return output{}, fmt.Errorf("Prometheus query error %s: %s", envelope.ErrorType, envelope.Error)
		}
		return output{}, fmt.Errorf("Prometheus query status %q", envelope.Status)
	}
	var data queryData
	if err := strictjson.Unmarshal(envelope.Data, &data); err != nil {
		return output{}, fmt.Errorf("decode Prometheus query data: %w", err)
	}
	if !validResultType(data.ResultType) {
		return output{}, fmt.Errorf("Prometheus resultType %q is unsupported", data.ResultType)
	}
	if len(data.Result) == 0 {
		return output{}, errors.New("Prometheus query result is empty")
	}
	return output{
		Tool:       "metric_query",
		Source:     "prometheus",
		Query:      cfg.Query,
		Time:       cfg.Time,
		ResultType: data.ResultType,
		Result:     append(json.RawMessage(nil), data.Result...),
		Warnings:   append([]string(nil), envelope.Warnings...),
		Infos:      append([]string(nil), envelope.Infos...),
	}, nil
}

func validResultType(value string) bool {
	switch value {
	case "vector", "matrix", "scalar", "string":
		return true
	default:
		return false
	}
}

func readCapped(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read Prometheus response: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("Prometheus response exceeds maximum %d bytes", maxBytes)
	}
	return body, nil
}

func formatDurationSeconds(d time.Duration) string {
	seconds := d.Seconds()
	if seconds == float64(int64(seconds)) {
		return strconv.FormatInt(int64(seconds), 10) + "s"
	}
	return strconv.FormatFloat(seconds, 'f', 3, 64) + "s"
}
