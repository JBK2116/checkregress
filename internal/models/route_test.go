package models

import (
	"slices"
	"strings"
	"testing"
)

// validRawRoute returns a RawRoute with the minimum fields required to pass
// Validate, so path-specific tests only need to set Paths.
func validRawRoute() RawRoute {
	return RawRoute{
		ServerName: "primary",
		Legacy:     "https://legacy.example.com",
		Candidate:  "https://candidate.example.com",
	}
}

// catchPanic runs fn and returns the panic value as a string, failing the test
// if no panic occurs or the panic value is not a string.
func catchPanic(t *testing.T, fn func()) string {
	t.Helper()
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		fn()
	}()
	if recovered == nil {
		t.Fatal("expected panic, but no panic occurred")
	}
	msg, ok := recovered.(string)
	if !ok {
		t.Fatalf("expected string panic, got %T: %v", recovered, recovered)
	}
	return msg
}

func TestRawRouteValidateValidPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		paths []string
	}{
		{
			name:  "method and wildcard",
			paths: []string{"GET /api/users/{id}"},
		},
		{
			name:  "post method and wildcard",
			paths: []string{"POST /api/orders/{id}"},
		},
		{
			name:  "path only",
			paths: []string{"/api/health"},
		},
		{
			name:  "rest wildcard",
			paths: []string{"/api/files/{path...}"},
		},
		{
			name:  "root wildcard",
			paths: []string{"/api/status/{$}"},
		},
		{
			name:  "multiple paths together",
			paths: []string{"GET /api/users/{id}", "POST /api/orders/{id}", "/api/health"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := validRawRoute()
			r.Paths = tt.paths

			got := r.Validate()

			if !slices.Equal(got.Paths, tt.paths) {
				t.Errorf("Paths = %v, want %v", got.Paths, tt.paths)
			}
		})
	}
}

func TestRawRouteValidateInvalidPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
	}{
		{
			name: "missing leading slash",
			path: "api/users",
		},
		{
			name: "bad wildcard name",
			path: "GET /api/users/{user-id}",
		},
		{
			name: "conflicting wildcards",
			path: "/{x}/{x...}",
		},
		{
			name: "root wildcard not last",
			path: "/{$}/foo",
		},
		{
			name: "empty string",
			path: "",
		},
		{
			name: "host pattern",
			path: "GET example.com/api/users/{id}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := validRawRoute()
			r.Paths = []string{tt.path}

			msg := catchPanic(t, func() { r.Validate() })
			if !strings.Contains(msg, "improperly configured") {
				t.Fatalf("panic message %q does not mention an invalid field", msg)
			}
			if !strings.Contains(msg, tt.path) {
				t.Fatalf("panic message %q does not mention the offending path %q", msg, tt.path)
			}
		})
	}
}

func TestRawRouteValidateEmptyPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		paths []string
	}{
		{name: "nil", paths: nil},
		{name: "empty slice", paths: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := validRawRoute()
			r.Paths = tt.paths

			got := r.Validate()

			if len(got.Paths) != 0 {
				t.Errorf("len(Paths) = %d, want 0", len(got.Paths))
			}
		})
	}
}

func TestRawRouteValidateConflictingPaths(t *testing.T) {
	t.Parallel()
	r := validRawRoute()
	// Both patterns match /api/users/posts and neither is more specific, so
	// registering them on the same mux must be rejected.
	r.Paths = []string{"/api/{x}/posts", "/api/users/{y}"}

	msg := catchPanic(t, func() { r.Validate() })
	if !strings.Contains(msg, "improperly configured") {
		t.Fatalf("panic message %q does not mention an invalid field", msg)
	}
}

func TestRawRouteValidateDuplicatePaths(t *testing.T) {
	t.Parallel()
	r := validRawRoute()
	r.Paths = []string{"GET /x", "GET /x"}

	msg := catchPanic(t, func() { r.Validate() })
	if !strings.Contains(msg, "improperly configured") {
		t.Fatalf("panic message %q does not mention an invalid field", msg)
	}
}
