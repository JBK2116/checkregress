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

// NewServer returns a Server that listens on addr and dispatches requests to handler.
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
