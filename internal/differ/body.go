package differ

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	"github.com/JBK2116/checkregress/internal/models"
)

// parseJSON parses the JSON body of a response.
func parseJSON(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber() // json.Number keeps big ints exact

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err // includes empty body (io.EOF)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data after JSON value")
	}

	return v, nil
}

// flatten recursively flattens a JSON object into a map[string]any keyed by
// JSON Pointer (RFC 6901) paths. Pointer tokens are escaped ("~" -> "~0",
// "/" -> "~1") so that every JSON value maps to a unique, unambiguous path.
func flatten(pointer string, v any, out map[string]any) {
	obj, ok := v.(map[string]any)
	if !ok || len(obj) == 0 {
		// leaf: string, json.Number, bool, nil, []any, or empty object.
		out[pointer] = v
		return
	}

	for k, child := range obj {
		flatten(pointer+"/"+escapePointerToken(k), child, out)
	}
}

// escapePointerToken escapes a single JSON Pointer reference token per RFC 6901:
// "~" is replaced with "~0" and "/" is replaced with "~1", in that order.
func escapePointerToken(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return token
}

// filterBody filters the bodies of the two responses.
//
// It removes the paths that are not present in both responses, and
// it removes the paths that are present in both responses but have
// different values.
//
// The returned value are the stable legacy bodies.
func filterBody(bodyLegacy, bodySecondary any) map[string]any {
	lp := make(map[string]any)
	sp := make(map[string]any)

	flatten("", bodyLegacy, lp)
	flatten("", bodySecondary, sp)

	for path, lv := range lp {
		sv, found := sp[path]

		if !found {
			// the path is not present in the secondary response, remove it as it is a dynamic field that changes per request.
			delete(lp, path)
			continue
		}
		if !reflect.DeepEqual(lv, sv) {
			// the path is present in both responses but differs in value, remove it as it is not stable.
			delete(lp, path)
		}
	}

	return lp
}

// diffBody compares the JSON bodies received in a legacy response and candidate response.
func diffBody(stable map[string]any, candidate any) map[string]models.Diff {
	cp := make(map[string]any)
	flatten("", candidate, cp)
	paths := make(map[string]models.Diff)

	for p, sv := range stable {
		cv, found := cp[p]

		if !found {
			d := models.Diff{
				Present:  false,
				Matching: false,
			}
			paths[p] = d
			continue
		}

		d := models.Diff{Present: found, Matching: reflect.DeepEqual(sv, cv)}
		paths[p] = d
	}
	return paths
}

// diffBytes checks if the provided payload bodies are byte-equivalent.
func diffBytes(bytesLegacy, bytesCandidate []byte) bool {
	return bytes.Equal(bytesLegacy, bytesCandidate)
}
