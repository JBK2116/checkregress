package differ

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/JBK2116/checkregress/internal/models"
)

// outcome builds an Outcome with the fields Diff reads (Status, Headers, Body).
func outcome(status int, headers http.Header, body string) models.Outcome {
	return models.Outcome{
		Status:  status,
		Headers: headers,
		Body:    []byte(body),
	}
}

func TestDiff(t *testing.T) {
	t.Parallel()

	jsonCT := http.Header{"Content-Type": {"application/json"}}
	textCT := http.Header{"Content-Type": {"text/plain"}}

	tests := []struct {
		name      string
		legacy    models.Outcome
		secondary models.Outcome
		candidate models.Outcome
		want      models.Result
	}{
		{
			name:      "status mismatch short-circuits",
			legacy:    outcome(200, jsonCT, `{"a":1}`),
			secondary: outcome(200, jsonCT, `{"a":1}`),
			candidate: outcome(404, jsonCT, `{"a":1}`),
			want:      models.Result{StatusMatch: false},
		},
		{
			name:      "status match with empty bodies diffs headers only",
			legacy:    outcome(200, jsonCT, ""),
			secondary: outcome(200, jsonCT, ""),
			candidate: outcome(200, jsonCT, ""),
			want: models.Result{
				StatusMatch: true,
				HeaderDiffs: models.HeaderDiffs{"Content-Type": {Present: true, Matching: true}},
				BodyDiffs: models.BodyDiffs{
					BytesMatch: true,
					Paths:      map[string]models.Diff{},
				},
			},
		},
		{
			name:      "status match with identical JSON bodies",
			legacy:    outcome(200, jsonCT, `{"a":1}`),
			secondary: outcome(200, jsonCT, `{"a":1}`),
			candidate: outcome(200, jsonCT, `{"a":1}`),
			want: models.Result{
				StatusMatch: true,
				HeaderDiffs: models.HeaderDiffs{"Content-Type": {Present: true, Matching: true}},
				BodyDiffs: models.BodyDiffs{
					BytesMatch: true,
					Paths:      map[string]models.Diff{"/a": {Present: true, Matching: true}},
				},
			},
		},
		{
			name:      "status match with differing JSON bodies",
			legacy:    outcome(200, jsonCT, `{"a":1,"b":2}`),
			secondary: outcome(200, jsonCT, `{"a":1,"b":2}`),
			candidate: outcome(200, jsonCT, `{"a":1,"b":3}`),
			want: models.Result{
				StatusMatch: true,
				HeaderDiffs: models.HeaderDiffs{"Content-Type": {Present: true, Matching: true}},
				BodyDiffs: models.BodyDiffs{
					BytesMatch: false,
					Paths: map[string]models.Diff{
						"/a": {Present: true, Matching: true},
						"/b": {Present: true, Matching: false},
					},
				},
			},
		},
		{
			name:      "status match with non-json bodies compares bytes only",
			legacy:    outcome(200, textCT, "hello"),
			secondary: outcome(200, textCT, "hello"),
			candidate: outcome(200, textCT, "world"),
			want: models.Result{
				StatusMatch: true,
				HeaderDiffs: models.HeaderDiffs{"Content-Type": {Present: true, Matching: true}},
				BodyDiffs: models.BodyDiffs{
					BytesMatch: false,
					Paths:      map[string]models.Diff{},
				},
			},
		},
		{
			name:      "status match with invalid json bodies compares bytes only",
			legacy:    outcome(200, jsonCT, "not json"),
			secondary: outcome(200, jsonCT, "not json"),
			candidate: outcome(200, jsonCT, "not json"),
			want: models.Result{
				StatusMatch: true,
				HeaderDiffs: models.HeaderDiffs{"Content-Type": {Present: true, Matching: true}},
				BodyDiffs: models.BodyDiffs{
					BytesMatch: true,
					Paths:      map[string]models.Diff{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := NewDiffer()
			got := d.Diff(tt.legacy, tt.secondary, tt.candidate)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Diff(...) = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestIsJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		headers http.Header
		want    bool
	}{
		{name: "application/json", headers: http.Header{"Content-Type": {"application/json"}}, want: true},
		{name: "json with charset", headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}, want: true},
		{name: "json subtype suffix", headers: http.Header{"Content-Type": {"application/ld+json"}}, want: true},
		{name: "plain text", headers: http.Header{"Content-Type": {"text/plain"}}, want: false},
		{name: "missing content-type", headers: http.Header{}, want: false},
		{name: "malformed content-type", headers: http.Header{"Content-Type": {"application/json; charset"}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isJSON(tt.headers); got != tt.want {
				t.Errorf("isJSON(%v) = %v, want %v", tt.headers, got, tt.want)
			}
		})
	}
}

func TestAllJSON(t *testing.T) {
	t.Parallel()
	jsonOut := models.Outcome{Headers: http.Header{"Content-Type": {"application/json"}}}
	textOut := models.Outcome{Headers: http.Header{"Content-Type": {"text/plain"}}}

	tests := []struct {
		name     string
		outcomes []models.Outcome
		want     bool
	}{
		{name: "all json", outcomes: []models.Outcome{jsonOut, jsonOut, jsonOut}, want: true},
		{name: "one not json", outcomes: []models.Outcome{jsonOut, textOut, jsonOut}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := allJSON(tt.outcomes...); got != tt.want {
				t.Errorf("allJSON(...) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllPayload(t *testing.T) {
	t.Parallel()
	withBody := models.Outcome{Body: []byte("x")}
	noBody := models.Outcome{}

	tests := []struct {
		name     string
		outcomes []models.Outcome
		want     bool
	}{
		{name: "all have payload", outcomes: []models.Outcome{withBody, withBody, withBody}, want: true},
		{name: "one empty", outcomes: []models.Outcome{withBody, noBody, withBody}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := allPayload(tt.outcomes...); got != tt.want {
				t.Errorf("allPayload(...) = %v, want %v", got, tt.want)
			}
		})
	}
}
