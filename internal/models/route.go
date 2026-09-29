package models

import (
	"fmt"
	"net/url"
)

// RawRoute represents a raw proxy route parsed from "config.yml".
type RawRoute struct {
	// Name sets the name of the routing group.
	Name string `yaml:"name"`
	// Legacy is the full url of the legacy endpoint.
	Legacy string `yaml:"legacy"`
	// Secondary is the full url of the shadow legacy endpoint.
	Secondary string `yaml:"secondary,omitempty"`
	// Candidate is the full url of the new migration endpoint.
	Candidate string `yaml:"candidate"`
}

// Route represents a proxy route in the application.
type Route struct {
	// Name sets the name of the routing group.
	Name string
	// Legacy is the full url of the legacy endpoint.
	Legacy *url.URL
	// Secondary is the full url of the shadow legacy endpoint.
	Secondary *url.URL
	// Candidate is the full url of the new migration endpoint.
	Candidate *url.URL
}

// Validate returns a validated proxy route.
func (r *RawRoute) Validate() Route {
	const missingFieldMessage = "invalid yaml configuration (missing required field)"
	const legacyMismatchMessage = "invalid yaml configuration (legacy cannot equal candidate)"
	const secondaryMismatchMessage = "invalid yaml configuration (secondary cannot equal candidate)"

	if r.Name == "" {
		panic(fmt.Sprintf("%s: name", missingFieldMessage))
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
		Name:      r.Name,
		Legacy:    parseRouteURL("legacy", r.Legacy),
		Candidate: parseRouteURL("candidate", r.Candidate),
	}

	if r.Secondary != "" {
		parsed.Secondary = parseRouteURL("secondary", r.Secondary)
	} else {
		// Default secondary to legacy. Copy the URL value so the two targets
		// remain independent.
		secondary := *parsed.Legacy
		parsed.Secondary = &secondary
	}
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
