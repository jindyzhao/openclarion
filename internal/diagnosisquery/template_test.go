package diagnosisquery

import "testing"

func TestExpandTemplateUsesSafeAlertValues(t *testing.T) {
	template := `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`
	got, ok := ExpandTemplate(template, Values{
		Labels: map[string]string{
			"ORACLE_SID": "sapprd1",
			"TABLESPACE": "PSAPSR3USR",
		},
	})
	if !ok {
		t.Fatal("ExpandTemplate returned false")
	}
	want := `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="sapprd1",TABLESPACE="PSAPSR3USR"}`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestExpandTemplateRejectsMissingOrUnsafeValues(t *testing.T) {
	template := `up{job="{{label.job}}"}`
	tests := []struct {
		name   string
		values Values
	}{
		{
			name:   "missing",
			values: Values{Labels: map[string]string{}},
		},
		{
			name:   "quote",
			values: Values{Labels: map[string]string{"job": `api" or up`}},
		},
		{
			name:   "backslash",
			values: Values{Labels: map[string]string{"job": `api\prod`}},
		},
		{
			name:   "control",
			values: Values{Labels: map[string]string{"job": "api\nprod"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := ExpandTemplate(template, tc.values); ok {
				t.Fatalf("ExpandTemplate = %q, want rejected", got)
			}
		})
	}
}

func TestMatchesTemplateAcceptsOnlyTemplateShape(t *testing.T) {
	template := `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`
	if !MatchesTemplate(template, `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="sapprd1",TABLESPACE="PSAPSR3USR"}`) {
		t.Fatal("expected concrete query to match")
	}
	if MatchesTemplate(template, `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="sapprd1",TABLESPACE="PSAPSR3USR",pod="api-1"}`) {
		t.Fatal("unexpectedly matched query with extra matcher")
	}
	if MatchesTemplate(template, `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="sapprd1",TABLESPACE="PSAPSR3USR\" or up"}`) {
		t.Fatal("unexpectedly matched escaped quote injection")
	}
}

func TestMatchesTemplateRejectsInconsistentRepeatedPlaceholders(t *testing.T) {
	template := `sum(rate(container_cpu_usage_seconds_total{pod="{{label.pod}}"}[5m])) / sum(kube_pod_container_resource_limits{pod="{{label.pod}}"})`
	if !MatchesTemplate(template, `sum(rate(container_cpu_usage_seconds_total{pod="api-1"}[5m])) / sum(kube_pod_container_resource_limits{pod="api-1"})`) {
		t.Fatal("expected repeated placeholder with the same value to match")
	}
	if MatchesTemplate(template, `sum(rate(container_cpu_usage_seconds_total{pod="api-1"}[5m])) / sum(kube_pod_container_resource_limits{pod="api-2"})`) {
		t.Fatal("unexpectedly matched repeated placeholder with different values")
	}
}

func TestValidateTemplateRejectsUnsupportedPlaceholderForms(t *testing.T) {
	tests := []string{
		`up{job={{label.job}}}`,
		`up{job="{{label.job_name-extra}}"}`,
		`up{job="{{metadata.name}}"}`,
		`up{job="{{label.job}"}`,
		`up{job=~"{{label.job}}"}`,
		`up{job!~"{{label.job}}"}`,
		`label_replace(up, "dst", "{{label.job}}", "src", "(.*)")`,
	}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			if err := ValidateTemplate(query); err == nil {
				t.Fatal("ValidateTemplate returned nil")
			}
		})
	}
}

func TestResolveExecutableQueryRequiresConcreteQueryForParameterizedTemplates(t *testing.T) {
	template := `up{job="{{label.job}}"}`
	if got, ok := ResolveExecutableQuery(template, ""); ok {
		t.Fatalf("ResolveExecutableQuery = %q, want rejected", got)
	}
	got, ok := ResolveExecutableQuery(template, `up{job="prometheus"}`)
	if !ok || got != `up{job="prometheus"}` {
		t.Fatalf("ResolveExecutableQuery = %q/%v, want concrete query", got, ok)
	}
}

func TestResolveExecutableQueryAllowsStaticTemplateWithoutRequestedQuery(t *testing.T) {
	got, ok := ResolveExecutableQuery(`up`, "")
	if !ok || got != `up` {
		t.Fatalf("ResolveExecutableQuery = %q/%v, want static query", got, ok)
	}
	if got, ok := ResolveExecutableQuery(`up`, `process_start_time_seconds`); ok {
		t.Fatalf("ResolveExecutableQuery = %q, want rejected", got)
	}
}
