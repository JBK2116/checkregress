package differ

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/JBK2116/checkregress/internal/models"
)

func TestParseJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		want    any
		wantErr bool
	}{
		{
			name: "parses object with json.Number",
			in:   `{"a": 1}`,
			want: map[string]any{"a": json.Number("1")},
		},
		{
			name: "parses array",
			in:   `[1, 2, 3]`,
			want: []any{json.Number("1"), json.Number("2"), json.Number("3")},
		},
		{
			name: "preserves large integers exactly",
			in:   `12345678901234567890`,
			want: json.Number("12345678901234567890"),
		},
		{
			name: "parses null",
			in:   `null`,
			want: nil,
		},
		{
			name:    "rejects trailing data",
			in:      `{"a":1}{"b":2}`,
			wantErr: true,
		},
		{
			name:    "rejects empty body",
			in:      ``,
			wantErr: true,
		},
		{
			name:    "rejects whitespace only body",
			in:      `   `,
			wantErr: true,
		},
		{
			name:    "rejects invalid json",
			in:      `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseJSON([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseJSON(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseJSON(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFlatten(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   any
		want map[string]any
	}{
		{
			name: "nested object uses slash separated pointers",
			in:   map[string]any{"a": map[string]any{"b": 1}},
			want: map[string]any{"/a/b": 1},
		},
		{
			name: "dotted key does not collide with nested key",
			in:   map[string]any{"a.b": 1, "a": map[string]any{"b": 2}},
			want: map[string]any{"/a.b": 1, "/a/b": 2},
		},
		{
			name: "slash and tilde keys are escaped",
			in:   map[string]any{"a/b": 1, "m~n": 2},
			want: map[string]any{"/a~1b": 1, "/m~0n": 2},
		},
		{
			name: "array is a leaf",
			in:   map[string]any{"a": []any{1, 2}},
			want: map[string]any{"/a": []any{1, 2}},
		},
		{
			name: "empty object is a leaf",
			in:   map[string]any{"a": map[string]any{}},
			want: map[string]any{"/a": map[string]any{}},
		},
		{
			name: "top-level scalar uses empty pointer",
			in:   "hello",
			want: map[string]any{"": "hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := make(map[string]any)
			flatten("", tt.in, got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("flatten(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFilterBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		legacy    any
		secondary any
		want      map[string]any
	}{
		{
			name:      "keeps path present in both with equal value",
			legacy:    map[string]any{"a": 1},
			secondary: map[string]any{"a": 1},
			want:      map[string]any{"/a": 1},
		},
		{
			name:      "removes path present in both with differing value",
			legacy:    map[string]any{"a": 1},
			secondary: map[string]any{"a": 2},
			want:      map[string]any{},
		},
		{
			name:      "removes path present only in legacy",
			legacy:    map[string]any{"a": 1, "b": 2},
			secondary: map[string]any{"a": 1},
			want:      map[string]any{"/a": 1},
		},
		{
			name:      "ignores path present only in secondary",
			legacy:    map[string]any{"a": 1},
			secondary: map[string]any{"a": 1, "b": 2},
			want:      map[string]any{"/a": 1},
		},
		{
			name:      "keeps nested path with equal value",
			legacy:    map[string]any{"a": map[string]any{"b": 1}},
			secondary: map[string]any{"a": map[string]any{"b": 1}},
			want:      map[string]any{"/a/b": 1},
		},
		{
			name:      "removes nested path with differing value",
			legacy:    map[string]any{"a": map[string]any{"b": 1}},
			secondary: map[string]any{"a": map[string]any{"b": 2}},
			want:      map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterBody(tt.legacy, tt.secondary)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterBody(legacy, secondary) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiffBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		stable    map[string]any
		candidate any
		want      map[string]models.Diff
	}{
		{
			name:      "matching path present in both",
			stable:    map[string]any{"/a": 1},
			candidate: map[string]any{"a": 1},
			want:      map[string]models.Diff{"/a": {Present: true, Matching: true}},
		},
		{
			name:      "differing path present in both",
			stable:    map[string]any{"/a": 1},
			candidate: map[string]any{"a": 2},
			want:      map[string]models.Diff{"/a": {Present: true, Matching: false}},
		},
		{
			name:      "path present only in stable",
			stable:    map[string]any{"/a": 1},
			candidate: map[string]any{"b": 1},
			want:      map[string]models.Diff{"/a": {Present: false, Matching: false}},
		},
		{
			name:      "ignores path present only in candidate",
			stable:    map[string]any{"/a": 1},
			candidate: map[string]any{"a": 1, "b": 2},
			want:      map[string]models.Diff{"/a": {Present: true, Matching: true}},
		},
		{
			name:      "nested candidate is flattened internally",
			stable:    map[string]any{"/a/b": 1},
			candidate: map[string]any{"a": map[string]any{"b": 1}},
			want:      map[string]models.Diff{"/a/b": {Present: true, Matching: true}},
		},
		{
			name:      "empty stable returns empty diff",
			stable:    map[string]any{},
			candidate: map[string]any{"a": 1},
			want:      map[string]models.Diff{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := diffBody(tt.stable, tt.candidate)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("diffBody(stable, candidate) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiffBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		legacy    []byte
		candidate []byte
		want      bool
	}{
		{name: "equal bytes", legacy: []byte("abc"), candidate: []byte("abc"), want: true},
		{name: "differing bytes", legacy: []byte("abc"), candidate: []byte("abd"), want: false},
		{name: "both empty", legacy: []byte{}, candidate: []byte{}, want: true},
		{name: "one empty", legacy: []byte("abc"), candidate: []byte{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := diffBytes(tt.legacy, tt.candidate); got != tt.want {
				t.Errorf("diffBytes(%q, %q) = %v, want %v", tt.legacy, tt.candidate, got, tt.want)
			}
		})
	}
}
