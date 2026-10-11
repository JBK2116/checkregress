package models

import "time"

// Diff is the diff of a header or json path.
type Diff struct {
	// Present is true if the value is present in both responses.
	Present bool `json:"present"`
	// Matching is true if the values match in both responses.
	Matching bool `json:"matching"`
}

// HeaderDiffs is the JSON representation of the header column.
type HeaderDiffs map[string]Diff

// BodyDiffs is the JSON representation of the body column.
type BodyDiffs struct {
	BytesMatch bool            `json:"bytes_match"`
	Paths      map[string]Diff `json:"paths"`
}

// ShadowRow is a row in the shadows table.
type ShadowRow struct {
	// ID is the primary key.
	ID int64
	// CanonicalPath is the canonical path, e.g. "/api/users/{id}".
	CanonicalPath string
	// LegacyURL is the legacy url, e.g. "http://localhost:8080/api/users/20".
	LegacyURL string
	// SecondaryURL is the secondary url, e.g. "http://localhost:8081/api/users/20".
	SecondaryURL string
	// CandidateURL is the candidate url, e.g. "http://localhost:8082/api/users/20".
	CandidateURL string
	// CreatedAt is the timestamp of the shadow.
	CreatedAt time.Time
	// StatusLegacy is the status code of the legacy response.
	StatusLegacy int
	// StatusCandidate is the status code of the candidate response.
	StatusCandidate int
	// StatusMatch is true if the status codes match.
	StatusMatch bool
	// Headers are the headers diff data in JSON format.
	Headers HeaderDiffs
	// Body is the body diff data in JSON format.
	Body *BodyDiffs
}
