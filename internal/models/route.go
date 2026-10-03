package models

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// RawRoute represents a raw proxy route parsed from "config.yml".
type RawRoute struct {
	// ServerName is the hostname clients use to reach this route group. It
	// must match the incoming request's "Host" header (ignoring the port).
	ServerName string `yaml:"server_name"`
	// Legacy is the full url of the legacy endpoint.
	Legacy string `yaml:"legacy"`
	// Secondary is the full url of the shadow legacy endpoint.
	Secondary string `yaml:"secondary,omitempty"`
	// Candidate is the full url of the new migration endpoint.
	Candidate string `yaml:"candidate"`
	// Paths is the list of http.ServeMux patterns (e.g. "GET /api/users/{id}")
	// this route group shadows. Empty means no shadowing.
	Paths []string `yaml:"paths,omitempty"`
}

// Route represents a proxy route in the application.
type Route struct {
	// ServerName is the hostname clients use to reach this route group. It
	// must match the incoming request's "Host" header (ignoring the port).
	ServerName string
	// Legacy is the full url of the legacy endpoint.
	Legacy *url.URL
	// Secondary is the full url of the shadow legacy endpoint.
	Secondary *url.URL
	// Candidate is the full url of the new migration endpoint.
	Candidate *url.URL
	// Paths is the validated list of http.ServeMux patterns for this route group.
	Paths []string
}

// Validate returns a validated proxy route.
func (r *RawRoute) Validate() Route {
	const missingFieldMessage = "invalid yaml configuration (missing required field)"
	const legacyMismatchMessage = "invalid yaml configuration (legacy cannot equal candidate)"
	const secondaryMismatchMessage = "invalid yaml configuration (secondary cannot equal candidate)"
	const invalidPathMessage = "invalid yaml configuration (field is improperly configured)"

	if r.ServerName == "" {
		panic(fmt.Sprintf("%s: server_name", missingFieldMessage))
	}
	if r.Legacy == "" {
		panic(fmt.Sprintf("%s: legacy", missingFieldMessage))
	}
	if r.Candidate == "" {
		panic(fmt.Sprintf("%s: candidate", missingFieldMessage))
	}
	if r.Legacy == r.Candidate {
		panic(fmt.Sprintf("%s: (legacy = %s) (candidate = %s)", legacyMismatchMessage, r.Legacy, r.Candidate))
	}
	if r.Secondary != "" && r.Secondary == r.Candidate {
		panic(fmt.Sprintf("%s: (secondary = %s) (candidate = %s)", secondaryMismatchMessage, r.Secondary, r.Candidate))
	}

	parsed := Route{
		ServerName: r.ServerName,
		Legacy:     parseRouteURL("legacy", r.Legacy),
		Candidate:  parseRouteURL("candidate", r.Candidate),
	}

	if r.Secondary != "" {
		parsed.Secondary = parseRouteURL("secondary", r.Secondary)
	} else {
		// Default secondary to legacy. Copy the URL value so the two targets
		// remain independent.
		secondary := *parsed.Legacy
		parsed.Secondary = &secondary
	}

	for _, p := range r.Paths {
		if !isValidPattern(p) {
			panic(fmt.Sprintf("%s: path (%s)", invalidPathMessage, p))
		}
		if isHostPattern(p) {
			panic(fmt.Sprintf("%s: path (%s) must not include a host", invalidPathMessage, p))
		}
	}
	if !isValidPaths(r.Paths) {
		panic(fmt.Sprintf("%s: paths (%s)", invalidPathMessage, strings.Join(r.Paths, ", ")))
	}

	// Copy the slice so Route.Paths is independent of RawRoute.Paths.
	parsed.Paths = append([]string(nil), r.Paths...)
	return parsed
}

// parseRouteURL parses a raw endpoint string into an absolute URL and panics
// with a descriptive message if the value is malformed or lacks a scheme or
// host.
func parseRouteURL(field, raw string) *url.URL {
	const invalidFieldMessage = "invalid yaml configuration (field is improperly configured)"

	u, err := url.ParseRequestURI(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		panic(fmt.Sprintf("%s: %s (%s)", invalidFieldMessage, field, raw))
	}
	return u
}

// isValidPattern reports whether pattern is a syntactically valid
// [http.ServeMux] pattern by attempting to register it on a throwaway mux.
// ServeMux panics on invalid patterns; the deferred recover swallows that
// panic, leaving the result at its zero value (false).
func isValidPattern(pattern string) bool {
	defer func() { recover() }()
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	return true
}

// isValidPaths reports whether every pattern can be registered on the same
// [http.ServeMux]. In addition to per-pattern syntax, this catches conflicts
// (two overlapping patterns where neither is more specific) and exact
// duplicates, both of which make ServeMux panic. The deferred recover
// swallows the panic, leaving the result at its zero value (false).
func isValidPaths(paths []string) bool {
	defer func() { recover() }()
	mux := http.NewServeMux()
	for _, p := range paths {
		mux.HandleFunc(p, func(http.ResponseWriter, *http.Request) {})
	}
	return true
}

// isHostPattern reports whether pattern contains a host component. A ServeMux
// pattern is host-based unless it starts with "/" (path-only) or with
// "<METHOD> " (method-prefixed path). Hosts are disallowed because routing
// already keys off the route's ServerName, so a host inside a path pattern is
// almost certainly a mistake.
func isHostPattern(pattern string) bool {
	if strings.HasPrefix(pattern, "/") {
		return false
	}
	if _, path, ok := strings.Cut(pattern, " "); ok {
		return !strings.HasPrefix(path, "/")
	}
	return true
}
