package differ

import (
	"net/http"
	"slices"
	"strings"

	"github.com/JBK2116/checkregress/internal/models"
)

// This file contains the code to compare the headers of two HTTP responses.

// commaSplitHeaders lists response headers whose values are comma-separated lists.
// The value says whether order is irrelevant (true = sort before comparing).
// Keys are in canonical form, matching [http.Header] keys.
var commaSplitHeaders = map[string]bool{
	// order-insensitive
	"Cache-Control":                 true,
	"Vary":                          true,
	"Allow":                         true,
	"Pragma":                        true,
	"Connection":                    true,
	"Content-Language":              true,
	"Accept-Ranges":                 true,
	"Accept-Ch":                     true,
	"Access-Control-Allow-Methods":  true,
	"Access-Control-Allow-Headers":  true,
	"Access-Control-Expose-Headers": true,
	"Timing-Allow-Origin":           true,

	// order-sensitive
	"Content-Encoding": false, // order of applied encodings
	"Via":              false, // proxy chain order
	"Upgrade":          false, // protocol preference
	"Alt-Svc":          false, // preference order
	"Server-Timing":    false,
}

// splitHeaders splits the header values by comma.
func splitHeaders(value []string, sortValues bool) []string {
	var out []string
	for _, v := range value {
		split := strings.SplitSeq(v, ",")
		for s := range split {
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
	}
	if sortValues {
		slices.Sort(out)
	}
	return out
}

// filterHeaders filters the headers of the two responses.
//
// It removes the headers that are not present in both responses, and
// it removes the headers that are present in both responses but have
// different values.
//
// The returned value are the stable legacy headers.
func filterHeaders(headLegacy, headSecondary http.Header) http.Header {
	out := headLegacy.Clone()
	for k, v := range headLegacy {
		sv, found := headSecondary[k]
		// If the header is not present in the secondary response, remove it as it is a dynamic field that changes per request.
		if !found {
			delete(out, k)
			continue
		}

		sortValues, found := commaSplitHeaders[k]

		// If the header is not present in the commaSplitHeaders, check the values directly.
		if !found {
			if !slices.Equal(v, sv) {
				delete(out, k)
			}
			continue
		}

		splitV := splitHeaders(v, sortValues)
		splitSV := splitHeaders(sv, sortValues)

		// if the values are not equal, remove the header as it is not stable.
		if !slices.Equal(splitV, splitSV) {
			delete(out, k)
		}
	}
	return out
}

// diffHeaders compares the headers received in a legacy response and candidate response.
func diffHeaders(headLegacy http.Header, headCandidate http.Header) models.HeaderDiffs {
	out := models.HeaderDiffs{}
	for k, v := range headLegacy {
		cv, found := headCandidate[k]

		if !found {
			out[k] = models.Diff{
				Present:  false,
				Matching: false,
			}
			continue
		}

		sortValues, ok := commaSplitHeaders[k]
		if !ok {
			out[k] = models.Diff{Present: found, Matching: slices.Equal(v, cv)}
			continue
		}

		splitV := splitHeaders(v, sortValues)
		splitCV := splitHeaders(cv, sortValues)

		out[k] = models.Diff{Present: found, Matching: slices.Equal(splitV, splitCV)}
	}
	return out
}
