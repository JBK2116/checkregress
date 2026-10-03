package models

import (
	"net/http"
)

// OutcomeErrorType categorizes the error types for outbound requests to backends.
type OutcomeErrorType int

const (
	// OutcomeErrorNone indicates that the request resulted with no errors.
	OutcomeErrorNone OutcomeErrorType = iota
	// ErrTimeout indicates the request exceeded the timeout length.
	ErrTimeout
	// ErrTransport indicates the request could not reach its backend
	// (DNS, connection refused, TLS, reset mid-request, etc...).
	ErrTransport
	// ErrNotFound indicates that the backend responded to the request with HTTP 404.
	ErrNotFound
	// ErrClient indicates the backend responded to the request with HTTP 4xx.
	ErrClient
	// ErrServer indicates the backend responded to the request with HTTP 5xx.
	ErrServer
)

// OutcomeShadowType determines which URL the outcome is received from.
type OutcomeShadowType int

const (
	// OutcomeLegacy maps to a proxy forwarded to the legacy url.
	OutcomeLegacy OutcomeShadowType = iota
	// OutcomeSecondary maps to a shadow request sent to the secondary url.
	OutcomeSecondary
	// OutcomeCandidate maps to a shadow request sent to the candidate url.
	OutcomeCandidate
)

// Outcome represents a result from a request sent to an external backend.
type Outcome struct {
	// Type states the source of the Outcome (legacy, secondary or candidate).
	Type OutcomeShadowType
	// CanonicalPath holds the canonical route pattern (e.g. "GET /api/users/{id}")
	// that matched the request, used to bucket outcomes for diffing and storage.
	CanonicalPath string
	// Status stores the HTTP status result of the request.
	Status int
	// Headers stores the headers received from the request.
	Headers http.Header
	// Body stores the body received from the request.
	Body []byte
	// Error stores any error that happened during the request lifecycle.
	Error OutcomeErrorType
}
