package proxy

import (
	"net/http"
	"time"
)

// Server handles incoming http requests.
type Server struct {
	// srv provides functionality.
	srv *http.Server
}

// NewServer returns a Server that listens on addr and dispatches requests to
// handler. readHeaderTimeout bounds how long the server waits for a client's
// request headers, and idleTimeout bounds how long an idle keep-alive
// connection stays open. ReadTimeout and WriteTimeout are intentionally left
// at zero so that slow or streaming requests to the proxied application are
// never truncated.
func NewServer(addr string, handler http.Handler, readHeaderTimeout, idleTimeout time.Duration) Server {
	return Server{
		srv: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			IdleTimeout:       idleTimeout,
		},
	}
}

// ListenAndServe listens on the server's configured address and serves incoming
// requests. It returns http.ErrServerClosed on a clean shutdown.
func (s Server) ListenAndServe() error {
	return s.srv.ListenAndServe()
}
