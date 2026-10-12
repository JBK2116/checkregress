package differ

import (
	"bytes"
	"mime"
	"net/http"
	"strings"

	"github.com/JBK2116/checkregress/internal/models"
)

// Differ provides functionality for comparing the outcomes between a request
// to legacy and candidate.
type Differ struct {
}

// NewDiffer returns a new Differ.
func NewDiffer() Differ {
	return Differ{}
}

// Diff gathers the resulting data from the outcomes of a request to legacy, secondary, and candidate.
func (d *Differ) Diff(legacy, secondary, candidate models.Outcome) models.Result {
	out := models.Result{
		StatusMatch: legacy.Status == candidate.Status,
	}

	// Headers and Body stay nil on a status mismatch (row constraint).
	if !out.StatusMatch {
		return out
	}

	out.HeaderDiffs = diffHeaders(filterHeaders(legacy.Headers, secondary.Headers), candidate.Headers)

	body := models.BodyDiffs{
		BytesMatch: bytes.Equal(legacy.Body, candidate.Body),
		Paths:      map[string]models.Diff{},
	}

	if allPayload(legacy, secondary, candidate) && allJSON(legacy, secondary, candidate) {
		lv, lerr := parseJSON(legacy.Body)
		sv, serr := parseJSON(secondary.Body)
		cv, cerr := parseJSON(candidate.Body)
		// include the body diff iff the JSONs are in a valid format.
		if lerr == nil && serr == nil && cerr == nil {
			body.Paths = diffBody(filterBody(lv, sv), cv)
		}
	}
	out.BodyDiffs = body

	return out
}


// allPayload checks if all outcomes have a payload attached.
func allPayload(outcomes ...models.Outcome) bool {
	for _, o := range outcomes {
		if len(o.Body) == 0 {
			return false
		}
	}
	return true
}

// isJSON checks if the Content-Type header is JSON.
func isJSON(h http.Header) bool {
	mt, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

// allJSON checks if all outcomes have a JSON Content-Type header.
func allJSON(outcomes ...models.Outcome) bool {
	for _, o := range outcomes {
		if !isJSON(o.Headers) {
			return false
		}
	}
	return true
}
