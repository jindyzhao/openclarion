package strictjson

import (
	"strings"
	"testing"
)

func TestUnmarshalRejectsDuplicateObjectKeys(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "top level duplicate",
			raw:  `{"schema":"old","schema":"new"}`,
			want: `duplicate object key "schema"`,
		},
		{
			name: "nested duplicate",
			raw:  `{"evidence":{"status":"pass","status":"fail"}}`,
			want: `$.evidence: duplicate object key "status"`,
		},
		{
			name: "array nested duplicate",
			raw:  `{"cases":[{"id":"one","id":"two"}]}`,
			want: `$.cases[0]`,
		},
		{
			name: "trailing value",
			raw:  `{"passed":true} {"passed":false}`,
			want: "trailing JSON values",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out map[string]any
			err := Unmarshal([]byte(tt.raw), &out)
			if err == nil {
				t.Fatal("Unmarshal err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Unmarshal err = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestUnmarshalAcceptsDistinctKeys(t *testing.T) {
	raw := []byte(`{"schema":"openclarion_sub_report","cases":[{"id":"one"},{"id":"two"}]}`)
	var out struct {
		Schema string `json:"schema"`
		Cases  []struct {
			ID string `json:"id"`
		} `json:"cases"`
	}
	if err := Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Schema != "openclarion_sub_report" || len(out.Cases) != 2 {
		t.Fatalf("decoded output = %+v", out)
	}
}

func TestUnmarshalRejectsUnknownStructFields(t *testing.T) {
	raw := []byte(`{"schema":"openclarion_sub_report","unexpected":true}`)
	var out struct {
		Schema string `json:"schema"`
	}
	err := Unmarshal(raw, &out)
	if err == nil {
		t.Fatal("Unmarshal err = nil, want unknown field error")
	}
	if !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("Unmarshal err = %q, want unknown field error", err.Error())
	}
}
