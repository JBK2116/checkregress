package differ

import (
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/JBK2116/checkregress/internal/models"
)

func TestSplitHeaders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		value      []string
		sortValues bool
		want       []string
	}{
		{
			name:       "splits comma separated values",
			value:      []string{"a, b"},
			sortValues: false,
			want:       []string{"a", "b"},
		},
		{
			name:       "trims whitespace and drops empty elements",
			value:      []string{" a ,  b ,, c "},
			sortValues: false,
			want:       []string{"a", "b", "c"},
		},
		{
			name:       "flattens values across slice elements",
			value:      []string{"a, b", "c"},
			sortValues: false,
			want:       []string{"a", "b", "c"},
		},
		{
			name:       "sorts when sortValues is true",
			value:      []string{"z, a, m"},
			sortValues: true,
			want:       []string{"a", "m", "z"},
		},
		{
			name:       "preserves order when sortValues is false",
			value:      []string{"z, a, m"},
			sortValues: false,
			want:       []string{"z", "a", "m"},
		},
		{
			name:       "empty input",
			value:      []string{},
			sortValues: false,
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := splitHeaders(tt.value, tt.sortValues)
			if !slices.Equal(got, tt.want) {
				t.Errorf("splitHeaders(%v, %v) = %v, want %v", tt.value, tt.sortValues, got, tt.want)
			}
		})
	}
}

func TestFilterHeaders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		legacy    http.Header
		secondary http.Header
		want      http.Header
	}{
		{
			name:      "keeps header present in both with equal value",
			legacy:    http.Header{"Content-Type": {"application/json"}},
			secondary: http.Header{"Content-Type": {"application/json"}},
			want:      http.Header{"Content-Type": {"application/json"}},
		},
		{
			name:      "removes header present in both with differing value",
			legacy:    http.Header{"Content-Type": {"application/json"}},
			secondary: http.Header{"Content-Type": {"text/plain"}},
			want:      http.Header{},
		},
		{
			name:      "removes header present only in legacy",
			legacy:    http.Header{"X-Dynamic": {"123"}},
			secondary: http.Header{},
			want:      http.Header{},
		},
		{
			name:      "ignores header present only in secondary",
			legacy:    http.Header{"Content-Type": {"application/json"}},
			secondary: http.Header{"Content-Type": {"application/json"}, "X-Extra": {"1"}},
			want:      http.Header{"Content-Type": {"application/json"}},
		},
		{
			name:      "keeps order-insensitive comma header with reordered equal values",
			legacy:    http.Header{"Vary": {"Accept-Encoding, Origin"}},
			secondary: http.Header{"Vary": {"Origin, Accept-Encoding"}},
			want:      http.Header{"Vary": {"Accept-Encoding, Origin"}},
		},
		{
			name:      "removes order-insensitive comma header with differing values",
			legacy:    http.Header{"Vary": {"Accept-Encoding, Origin"}},
			secondary: http.Header{"Vary": {"Accept-Encoding"}},
			want:      http.Header{},
		},
		{
			name:      "keeps order-sensitive comma header with matching order",
			legacy:    http.Header{"Content-Encoding": {"gzip, br"}},
			secondary: http.Header{"Content-Encoding": {"gzip, br"}},
			want:      http.Header{"Content-Encoding": {"gzip, br"}},
		},
		{
			name:      "removes order-sensitive comma header with reordered values",
			legacy:    http.Header{"Content-Encoding": {"gzip, br"}},
			secondary: http.Header{"Content-Encoding": {"br, gzip"}},
			want:      http.Header{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterHeaders(tt.legacy, tt.secondary)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterHeaders(legacy, secondary) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiffHeaders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		legacy    http.Header
		candidate http.Header
		want      models.HeaderDiffs
	}{
		{
			name:      "matching header present in both",
			legacy:    http.Header{"Content-Type": {"application/json"}},
			candidate: http.Header{"Content-Type": {"application/json"}},
			want:      models.HeaderDiffs{"Content-Type": {Present: true, Matching: true}},
		},
		{
			name:      "differing header present in both",
			legacy:    http.Header{"Content-Type": {"application/json"}},
			candidate: http.Header{"Content-Type": {"text/plain"}},
			want:      models.HeaderDiffs{"Content-Type": {Present: true, Matching: false}},
		},
		{
			name:      "header present only in legacy",
			legacy:    http.Header{"X-Dynamic": {"123"}},
			candidate: http.Header{},
			want:      models.HeaderDiffs{"X-Dynamic": {Present: false, Matching: false}},
		},
		{
			name:      "ignores header present only in candidate",
			legacy:    http.Header{"Content-Type": {"application/json"}},
			candidate: http.Header{"Content-Type": {"application/json"}, "X-Extra": {"1"}},
			want:      models.HeaderDiffs{"Content-Type": {Present: true, Matching: true}},
		},
		{
			name:      "order-insensitive comma header matches when reordered",
			legacy:    http.Header{"Vary": {"Accept-Encoding, Origin"}},
			candidate: http.Header{"Vary": {"Origin, Accept-Encoding"}},
			want:      models.HeaderDiffs{"Vary": {Present: true, Matching: true}},
		},
		{
			name:      "order-insensitive comma header mismatches on differing values",
			legacy:    http.Header{"Vary": {"Accept-Encoding, Origin"}},
			candidate: http.Header{"Vary": {"Accept-Encoding"}},
			want:      models.HeaderDiffs{"Vary": {Present: true, Matching: false}},
		},
		{
			name:      "order-sensitive comma header matches in same order",
			legacy:    http.Header{"Content-Encoding": {"gzip, br"}},
			candidate: http.Header{"Content-Encoding": {"gzip, br"}},
			want:      models.HeaderDiffs{"Content-Encoding": {Present: true, Matching: true}},
		},
		{
			name:      "order-sensitive comma header mismatches when reordered",
			legacy:    http.Header{"Content-Encoding": {"gzip, br"}},
			candidate: http.Header{"Content-Encoding": {"br, gzip"}},
			want:      models.HeaderDiffs{"Content-Encoding": {Present: true, Matching: false}},
		},
		{
			name:      "empty legacy returns empty diff",
			legacy:    http.Header{},
			candidate: http.Header{"Content-Type": {"application/json"}},
			want:      models.HeaderDiffs{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := diffHeaders(tt.legacy, tt.candidate)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("diffHeaders(legacy, candidate) = %v, want %v", got, tt.want)
			}
		})
	}
}
