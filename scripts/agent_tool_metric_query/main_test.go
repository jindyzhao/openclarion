package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunQueriesPrometheusInstantEndpoint(t *testing.T) {
	var gotQuery string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("query")
		if r.URL.Query().Get("time") != "2026-05-28T10:00:00Z" {
			t.Fatalf("time = %q", r.URL.Query().Get("time"))
		}
		if r.URL.Query().Get("timeout") != "2s" {
			t.Fatalf("timeout = %q", r.URL.Query().Get("timeout"))
		}
		if r.URL.Query().Get("limit") != "7" {
			t.Fatalf("limit = %q", r.URL.Query().Get("limit"))
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":[{"metric":{"job":"api"},"value":[1435781451.781,"1"]}]
			},
			"warnings":["partial warning"],
			"infos":["query info"]
		}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	err := run([]string{
		"--query", `up{job="api"}`,
		"--time", "2026-05-28T10:00:00Z",
		"--query-timeout", "2s",
		"--limit", "7",
	}, &stdout, mapGetenv(map[string]string{
		envPrometheusURL: server.URL,
		envBearerToken:   "test-bearer-value",
	}), server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotQuery != `up{job="api"}` {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotAuth != "Bearer test-bearer-value" {
		t.Fatalf("Authorization = %q", gotAuth)
	}

	var out output
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout JSON: %v\n%s", err, stdout.String())
	}
	if out.Tool != "metric_query" || out.Source != "prometheus" {
		t.Fatalf("unexpected tool/source: %#v", out)
	}
	if out.ResultType != "vector" {
		t.Fatalf("ResultType = %q", out.ResultType)
	}
	if !strings.Contains(string(out.Result), `"job": "api"`) {
		t.Fatalf("Result = %s", out.Result)
	}
	if len(out.Warnings) != 1 || len(out.Infos) != 1 {
		t.Fatalf("warnings/infos = %#v / %#v", out.Warnings, out.Infos)
	}
}

func TestParseConfigRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantErr string
	}{
		{name: "missing url", args: []string{"--query", "up"}, wantErr: envPrometheusURL},
		{name: "missing query", env: map[string]string{envPrometheusURL: "http://example.test"}, wantErr: "--query"},
		{name: "bad scheme", args: []string{"--query", "up"}, env: map[string]string{envPrometheusURL: "file:///tmp/prom"}, wantErr: "http or https"},
		{name: "userinfo", args: []string{"--query", "up"}, env: map[string]string{envPrometheusURL: "http://user@example.test"}, wantErr: "userinfo"},
		{name: "bad limit", args: []string{"--query", "up", "--limit", "0"}, env: map[string]string{envPrometheusURL: "http://example.test"}, wantErr: "limit"},
		{name: "bad http timeout", args: []string{"--query", "up", "--http-timeout", "0s"}, env: map[string]string{envPrometheusURL: "http://example.test"}, wantErr: "http-timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig(tt.args, mapGetenv(tt.env))
			if err == nil {
				t.Fatal("parseConfig err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseConfig err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParsePrometheusOutputRejectsErrors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "api error", body: `{"status":"error","errorType":"bad_data","error":"invalid query"}`, wantErr: "invalid query"},
		{name: "bad result type", body: `{"status":"success","data":{"resultType":"table","result":[]}}`, wantErr: "unsupported"},
		{name: "missing result", body: `{"status":"success","data":{"resultType":"vector"}}`, wantErr: "result is empty"},
		{name: "trailing json", body: `{"status":"success","data":{"resultType":"vector","result":[]}} {}`, wantErr: "trailing"},
		{name: "duplicate envelope key", body: `{"status":"error","status":"success","data":{"resultType":"vector","result":[]}}`, wantErr: "duplicate object key"},
		{name: "duplicate data key", body: `{"status":"success","data":{"resultType":"matrix","resultType":"vector","result":[]}}`, wantErr: "duplicate object key"},
		{name: "unknown data field", body: `{"status":"success","data":{"resultType":"vector","result":[],"extra":true}}`, wantErr: "unknown field"},
	}
	cfg := config{Query: "up", QueryTimeout: time.Second, Limit: 1, HTTPTimeout: time.Second}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePrometheusOutput(cfg, []byte(tt.body))
			if err == nil {
				t.Fatal("parsePrometheusOutput err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parsePrometheusOutput err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestQueryPrometheusRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := queryPrometheus(contextBackground(), config{
		PrometheusURL: server.URL,
		Query:         "up",
		QueryTimeout:  time.Second,
		Limit:         1,
		HTTPTimeout:   time.Second,
	}, server.Client())
	if err == nil {
		t.Fatal("queryPrometheus err = nil, want error")
	}
	if !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("queryPrometheus err = %v", err)
	}
}

func mapGetenv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func contextBackground() context.Context {
	return context.Background()
}
