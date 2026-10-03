package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShouldShadow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		paths  []string
		method string
		target string
		want   bool
	}{
		{
			name:   "method and wildcard match",
			paths:  []string{"GET /api/users/{id}"},
			method: http.MethodGet,
			target: "/api/users/42",
			want:   true,
		},
		{
			name:   "wildcard matches exactly one segment",
			paths:  []string{"GET /api/users/{id}"},
			method: http.MethodGet,
			target: "/api/users/42/posts",
			want:   false,
		},
		{
			name:   "method is part of the pattern",
			paths:  []string{"GET /api/users/{id}"},
			method: http.MethodPost,
			target: "/api/users/42",
			want:   false,
		},
		{
			name:   "head matches get",
			paths:  []string{"GET /api/users/{id}"},
			method: http.MethodHead,
			target: "/api/users/42",
			want:   true,
		},
		{
			name:   "path only pattern matches any method",
			paths:  []string{"/api/health"},
			method: http.MethodPost,
			target: "/api/health",
			want:   true,
		},
		{
			name:   "rest wildcard matches multiple segments",
			paths:  []string{"/api/files/{path...}"},
			method: http.MethodGet,
			target: "/api/files/a/b/c",
			want:   true,
		},
		{
			name:   "root wildcard matches exact path",
			paths:  []string{"/api/status/{$}"},
			method: http.MethodGet,
			target: "/api/status",
			want:   true,
		},
		{
			name:   "query string is ignored when matching",
			paths:  []string{"GET /api/users/{id}"},
			method: http.MethodGet,
			target: "/api/users/42?expand=posts",
			want:   true,
		},
		{
			name:   "no matching pattern",
			paths:  []string{"GET /api/users/{id}"},
			method: http.MethodGet,
			target: "/api/orders/7",
			want:   false,
		},
		{
			name:   "match among multiple patterns",
			paths:  []string{"GET /api/users/{id}", "POST /api/orders/{id}", "/api/health"},
			method: http.MethodPost,
			target: "/api/orders/7",
			want:   true,
		},
		{
			name:   "nil paths match nothing",
			paths:  nil,
			method: http.MethodGet,
			target: "/api/users/42",
			want:   false,
		},
		{
			name:   "empty paths match nothing",
			paths:  []string{},
			method: http.MethodGet,
			target: "/api/users/42",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := NewMatcher(tt.paths)
			r := httptest.NewRequest(tt.method, tt.target, nil)

			if got, _ := m.ShouldShadow(r); got != tt.want {
				t.Errorf("ShouldShadow() = %v, want %v", got, tt.want)
			}
		})
	}
}
