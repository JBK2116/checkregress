package proxy

import (
	"log/slog"
	"net"
	"net/http"

	"github.com/JBK2116/checkregress/internal/models"
)

// Handler routes incoming HTTP requests to the reverse proxy configured for
// the matching route group.
type Handler struct {
	// handler dispatches each request to the proxy whose ServerName matches
	// the request's Host header.
	handler http.HandlerFunc
}

// NewHandler returns a Handler that proxies each incoming request to the route
// whose ServerName matches the request's "Host" header (ignoring the port).
func NewHandler(log *slog.Logger, routes []models.Route) Handler {
	proxies := make(map[string]ReverseProxy, len(routes))

	// build a proxy for each route group, keyed by its server name
	for _, r := range routes {
		pr := newReverseProxy(r, log)
		proxies[r.ServerName] = pr
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := getHostHeader(r)
		if val, ok := proxies[host]; ok {
			val.Proxy.ServeHTTP(w, r)
			return
		}
		http.Error(w, "unsupported service", http.StatusNotFound)
	})
	return Handler{handler: handler}
}

// ServeHTTP implements [http.Handler].
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler(w, r)
}

// getHostHeader returns the "Host" header value omitting the port.
func getHostHeader(r *http.Request) string {
	hostport := r.Host

	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = r.Host
	}
	return host
}
