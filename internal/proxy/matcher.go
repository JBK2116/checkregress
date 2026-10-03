package proxy

import "net/http"

// Matcher provides functionality for determining if an incoming request should be shadowed.
type Matcher struct {
	// mux provides functionality for the Matcher struct.
	mux *http.ServeMux
}

// NewMatcher returns a Matcher configured with the provided route data.
func NewMatcher(paths []string) Matcher {
	mux := http.NewServeMux()
	for _, p := range paths {
		mux.HandleFunc(p, func(http.ResponseWriter, *http.Request) {})
	}
	return Matcher{mux: mux}
}

// ShouldShadow determines if the incoming request matches an existing path that can be shadowed.
func (m *Matcher) ShouldShadow(r *http.Request) (bool, string) {
	_, pattern := m.mux.Handler(r)
	return pattern != "", pattern
}
