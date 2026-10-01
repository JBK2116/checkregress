package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JBK2116/checkregress/internal/models"
)

func TestHandlerRoutesToMatchingServerName(t *testing.T) {
	t.Parallel()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("legacy response"))
	}))
	t.Cleanup(backend.Close)

	route := models.Route{
		ServerName: "users-service",
		Legacy:     mustParseURL(t, backend.URL),
	}
	h := NewHandler(discardLogger(), []models.Route{route})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = "users-service"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "legacy response" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "legacy response")
	}
}

func TestHandlerReturnsNotFoundForUnknownServerName(t *testing.T) {
	t.Parallel()

	route := models.Route{
		ServerName: "users-service",
		Legacy:     mustParseURL(t, "http://legacy.example.com"),
	}
	h := NewHandler(discardLogger(), []models.Route{route})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Host = "unknown-service"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
