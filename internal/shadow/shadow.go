package shadow

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/JBK2116/checkregress/internal/models"
)

// Shadow provides functionality for shadowing the inbound legacy client request.
type Shadow struct {
	// logger is used for tracking information during the shadow process.
	logger *slog.Logger
	// client is used for shadowing legacy to secondary & candidate
	client *http.Client
}

// NewShadow returns a Shadower configured with the provided client.
func NewShadow(logger *slog.Logger, client *http.Client) *Shadow {
	return &Shadow{logger: logger, client: client}
}

// Fire shadows the incoming legacy request to the provided backend url
// and updates the provided outcome object.
func (s *Shadow) Fire(
	ctx context.Context,
	outcome *models.Outcome,
	backend *url.URL,
	inBody []byte,
	in *http.Request,
) {
	target := s.buildTarget(in, backend)
	outcome.URL = target.String()

	var rdr io.Reader = http.NoBody
	if len(inBody) > 0 {
		rdr = bytes.NewReader(inBody)
	}

	out, err := http.NewRequestWithContext(
		ctx,
		in.Method,
		target.String(),
		rdr) // #nosec G704 -> host pinned to config via buildTarget; only path/query derive from the request
	if err != nil {
		outcome.FromError(err)
		return
	}

	out.Header = in.Header.Clone()
	s.delHeaders(out)
	s.setXForwarded(in, out)

	resp, err := s.client.Do(
		out) // #nosec G704 -> see buildTarget: destination host is config-fixed, request supplies path only
	if err != nil {
		outcome.FromError(err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		outcome.FromError(err)
		return
	}
	outcome.FromResponse(resp, body)
}

// setXForwarded sets the X-Forwarded-For, X-Forwarded-Host, and
// X-Forwarded-Proto headers for the outbound request.
//
// This method mimics the one used in [http.ProxyRequest].
func (s *Shadow) setXForwarded(in *http.Request, out *http.Request) {
	out.Header.Del("Forwarded")
	out.Header.Del("X-Forwarded-For")
	out.Header.Del("X-Forwarded-Host")
	out.Header.Del("X-Forwarded-Proto")

	if clientIP, _, err := net.SplitHostPort(in.RemoteAddr); err == nil {
		out.Header.Set("X-Forwarded-For", clientIP)
	}
	out.Header.Set("X-Forwarded-Host", in.Host)
	if in.TLS == nil {
		out.Header.Set("X-Forwarded-Proto", "http")
	} else {
		out.Header.Set("X-Forwarded-Proto", "https")
	}
}

// delHeaders removes hop by hop headers from the provided shadow http request.
// This function follows the strip logic used in [http.ReverseProxy].
func (s *Shadow) delHeaders(r *http.Request) {
	var hopHeaders = []string{
		"Connection",
		"Proxy-Connection", // non-standard but still sent by libcurl and rejected by e.g. google
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",      // canonicalized version of "TE"
		"Trailer", // not Trailers per URL above; https://www.rfc-editor.org/errata_search.php?eid=4522
		"Transfer-Encoding",
		"Upgrade",
	}

	if c := r.Header.Get("Connection"); c != "" {
		for f := range strings.SplitSeq(c, ",") {
			if f = textproto.TrimString(f); f != "" {
				r.Header.Del(f)
			}
		}
	}
	for _, h := range hopHeaders {
		r.Header.Del(h)
	}
}

// buildTarget constructs the url used for a shadow request.
//
// The destination host is always taken from base (the configured route URL),
// never from the request: only Path, RawPath, RawQuery, and ForceQuery are
// copied from in.URL. The request therefore cannot influence where the
// shadow request is sent (SSRF-safe).
func (s *Shadow) buildTarget(in *http.Request, base *url.URL) url.URL {
	target := *base
	target.Path = in.URL.Path
	target.RawPath = in.URL.RawPath
	target.RawQuery = in.URL.RawQuery
	target.ForceQuery = in.URL.ForceQuery
	return target
}
