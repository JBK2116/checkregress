package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"

	"github.com/JBK2116/checkregress/internal/models"
)

// ReverseProxy handles routing the client request to the correct backend server.
type ReverseProxy struct {
	// Proxy provides functionality for the struct.
	Proxy *httputil.ReverseProxy
}

// newReverseProxy returns a new ReverseProxy configured with the provided route data.
func newReverseProxy(route models.Route, log *slog.Logger) ReverseProxy {
	pr := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(route.Legacy)
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			log.ErrorContext(req.Context(), "proxy error",
				"method", req.Method,
				"path", req.URL.Path,
				"err", err,
			)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}
	return ReverseProxy{Proxy: pr}
}
