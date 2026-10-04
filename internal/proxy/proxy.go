package proxy

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
	// Proxy provides functionality for the struct.
	Proxy *httputil.ReverseProxy
}

// getOutcome retrieves the *Outcome object from the provided context.
func getOutcome(ctx context.Context) *models.Outcome {
	o, _ := ctx.Value(outcomeKey{}).(*models.Outcome)
	return o
}

// newReverseProxy returns a new ReverseProxy configured with the provided route data.
func newReverseProxy(route models.Route, log *slog.Logger) ReverseProxy {
	pr := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(route.Legacy)
			pr.SetXForwarded()
		},
		ModifyResponse: func(resp *http.Response) error {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				// route the proxy to ErrorHandler so the client is replied to properly.
				return err
			}
			resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(body))
			if o := getOutcome(resp.Request.Context()); o != nil {
				o.FromResponse(resp, body)
			}
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
	return ReverseProxy{Proxy: pr}
}

// Serve proxies the request and returns the outcome once it is done.
func (p *ReverseProxy) Serve(w http.ResponseWriter, r *http.Request, canonicalPath string) *models.Outcome {
	o := &models.Outcome{
		Type:          models.OutcomeLegacy,
		CanonicalPath: canonicalPath,
	}
	ctx := context.WithValue(r.Context(), outcomeKey{}, o)
	p.Proxy.ServeHTTP(w, r.WithContext(ctx))
	return o
}
