package network

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"

	"github.com/JBK2116/checkregress/internal/models"
)

// outcomeKey is used to access the *Outcome in the proxy context.
type outcomeKey struct{}

// ReverseProxy handles routing the client request to the correct backend server.
type ReverseProxy struct {
	// proxy provides functionality for the struct.
	proxy *httputil.ReverseProxy
}

// getOutcome retrieves the *Outcome object from the provided context.
func getOutcome(ctx context.Context) *models.Outcome {
	o, _ := ctx.Value(outcomeKey{}).(*models.Outcome)
	return o
}

// readCloser couples a reader with a separate closer so a re-wrapped response
// body can still close the original underlying body.
type readCloser struct {
	io.Reader
	io.Closer
}

// NewReverseProxy returns a new ReverseProxy configured with the provided route data.
func NewReverseProxy(route models.Route, log *slog.Logger, maxBodyBytes int) *ReverseProxy {
	pr := &httputil.ReverseProxy{
		Transport: newProxyTransport(),
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(route.Legacy)
			pr.SetXForwarded()
			if o := getOutcome(pr.Out.Context()); o != nil {
				o.URL = pr.Out.URL.String()
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			o := getOutcome(resp.Request.Context())
			if o == nil {
				return nil
			}
			// Capture only the first maxBodyBytes for the diff, but stream the
			// full response to the client.
			head, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBodyBytes)))
			if err != nil {
				// route the proxy to ErrorHandler so the client is replied too properly.
				resp.Body.Close()
				return err
			}
			resp.Body = &readCloser{
				Reader: io.MultiReader(bytes.NewReader(head), resp.Body),
				Closer: resp.Body,
			}
			o.FromResponse(resp, head)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			// record the error from the context as the response is not available.
			if o := getOutcome(req.Context()); o != nil {
				o.FromError(err)
			}
			log.ErrorContext(req.Context(), "proxy error",
				"method", req.Method,
				"path", req.URL.Path,
				"err", err,
			)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}
	return &ReverseProxy{proxy: pr}
}

// Serve proxies the request and returns the outcome once it is done.
func (p *ReverseProxy) Serve(w http.ResponseWriter, r *http.Request, canonicalPath string, shadow bool) *models.Outcome {
	if shadow {
		o := &models.Outcome{
			Type:          models.OutcomeLegacy,
			CanonicalPath: canonicalPath,
		}
		ctx := context.WithValue(r.Context(), outcomeKey{}, o)
		p.proxy.ServeHTTP(w, r.WithContext(ctx))
		return o
	}
	p.proxy.ServeHTTP(w, r)
	return nil
}

// CloseIdleConnections closes any connections on its Transport which were previously connected from previous requests but are now sitting idle in a "keep-alive" state.
func (p *ReverseProxy) CloseIdleConnections() {
	if t, ok := p.proxy.Transport.(interface{ CloseIdleConnections() }); ok {
		t.CloseIdleConnections()
	}
}
