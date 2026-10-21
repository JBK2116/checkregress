package orchestrator

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/JBK2116/checkregress/internal/differ"
	"github.com/JBK2116/checkregress/internal/models"
	"github.com/JBK2116/checkregress/internal/network"
	"github.com/JBK2116/checkregress/internal/shadow"
)

// shadowStore persists shadow rows. It is the subset of store.Store the
// Orchestrator depends on, kept as an interface so tests can inject a fake.
type shadowStore interface {
	AddRow(*models.ShadowRow)
}

// Orchestrator manages the request lifecycle from initial ingestion to persistence.
type Orchestrator struct {
	// logger is used for tracking the lifecycle via logs.
	logger *slog.Logger
	// matcher is used for determine if a request should be shadowed or just proxied.
	matcher *network.Matcher
	// shadow is used for shadowing the inbound request if it matches an allowlist path.
	shadow *shadow.Shadow
	// proxy forwards inbound requests to the legacy backend.
	proxy *network.ReverseProxy
	// differ is used for comparing the responses from the shadows and the candidate.
	differ *differ.Differ
	// store provides functionality for interacting with the database.
	store shadowStore
	// shadowTimeoutMS sets the max amount of milliseconds a worker has to shadow a proxy request before timing out.
	shadowTimeoutMS int
	// route is used for storing the route configuration.
	route models.Route
	// e2eTimeoutMS sets the max amount of milliseconds the proxy waits for the full end-to-end proxied request.
	e2eTimeoutMS int
	// maxBodyBytes defines the max amount of bytes to read from a response body.
	maxBodyBytes int
	// wg tracks in-flight differ/store goroutines so shutdown can drain them.
	wg sync.WaitGroup
}

// NewOrchestrator returns an Orchestrator configured with the provided values.
func NewOrchestrator(
	logger *slog.Logger,
	proxy *network.ReverseProxy,
	differ *differ.Differ,
	matcher *network.Matcher,
	shadow *shadow.Shadow,
	store shadowStore,
	route models.Route,
	shadowTimeoutMS int,
	e2eTimeoutMS int,
	maxBodyBytes int,
) *Orchestrator {
	return &Orchestrator{
		matcher:         matcher,
		logger:          logger,
		differ:          differ,
		shadow:          shadow,
		store:           store,
		route:           route,
		proxy:           proxy,
		shadowTimeoutMS: shadowTimeoutMS,
		e2eTimeoutMS:    e2eTimeoutMS,
		maxBodyBytes:    maxBodyBytes,
	}
}

// ServeHTTP manages the incoming requests lifecycle.
func (o *Orchestrator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(o.e2eTimeoutMS)*time.Millisecond)
	defer cancel()
	req := r.WithContext(ctx)

	shouldShadow, canonicalPath := o.matcher.ShouldShadow(r)
	if !shouldShadow {
		o.proxy.Serve(w, req, canonicalPath, false)
		return
	}

	body, err := io.ReadAll(req.Body)
	clErr := req.Body.Close()
	if clErr != nil {
		o.logger.ErrorContext(ctx, "failed to close request body", "err", clErr)
	}

	if err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}

	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))

	shadowCtx, shadowCancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		time.Duration(o.shadowTimeoutMS)*time.Millisecond,
	)

	var wg sync.WaitGroup
	secOutcome, candOutcome := models.Outcome{
		Type:          models.OutcomeSecondary,
		CanonicalPath: canonicalPath,
	}, models.Outcome{
		Type:          models.OutcomeCandidate,
		CanonicalPath: canonicalPath,
	}

	shadowCount := 2
	wg.Add(shadowCount)

	go func() {
		defer wg.Done()
		o.shadow.Fire(shadowCtx, &secOutcome, o.route.Secondary, body, req, o.maxBodyBytes)
	}()
	go func() {
		defer wg.Done()
		o.shadow.Fire(shadowCtx, &candOutcome, o.route.Candidate, body, req, o.maxBodyBytes)
	}()

	legOutcome := o.proxy.Serve(w, req, canonicalPath, true)

	o.wg.Go(func() {
		defer func() {
			if rec := recover(); rec != nil {
				o.logger.ErrorContext(ctx, "differ/store panic", "panic", rec)
			}
		}()
		defer shadowCancel()

		wg.Wait()

		if !shouldDiff(candOutcome, secOutcome, *legOutcome) {
			return
		}

		if !o.differ.QueueDiff() {
			return
		}
		defer func() {
			o.differ.DequeueDiff()
		}()

		res := o.differ.Diff(*legOutcome, secOutcome, candOutcome)
		row := genRow(*legOutcome, secOutcome, candOutcome, res)
		o.store.AddRow(&row)
	})
}

// Wait blocks until every in-flight differ/store goroutine has finished. It is
// called during graceful shutdown so no captured diff is lost when the store
// is closed.
func (o *Orchestrator) Wait() {
	o.wg.Wait()
}

// shouldDiff reports whether the outcomes should be diffed and stored. It
// returns false if any outcome failed to reach its backend (transport error or
// timeout), since there is no meaningful response to compare.
func shouldDiff(outcomes ...models.Outcome) bool {
	for _, o := range outcomes {
		if o.Error == models.ErrTransport || o.Error == models.ErrTimeout {
			return false
		}
	}
	return true
}

// genRow generates a shadow row from the outcomes and the result.
func genRow(legacy, secondary, candidate models.Outcome, result models.Result) models.ShadowRow {
	out := models.ShadowRow{
		CanonicalPath:   legacy.CanonicalPath,
		LegacyURL:       legacy.URL,
		SecondaryURL:    secondary.URL,
		CandidateURL:    candidate.URL,
		StatusLegacy:    legacy.Status,
		StatusCandidate: candidate.Status,
		StatusMatch:     result.StatusMatch,
		Headers:         result.HeaderDiffs,
	}
	if result.StatusMatch {
		out.Body = &result.BodyDiffs
	}
	return out
}
