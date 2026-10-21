package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JBK2116/checkregress/internal/config"
	"github.com/JBK2116/checkregress/internal/differ"
	"github.com/JBK2116/checkregress/internal/network"
	"github.com/JBK2116/checkregress/internal/orchestrator"
	"github.com/JBK2116/checkregress/internal/shadow"
	"github.com/JBK2116/checkregress/internal/store"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	yam := config.LoadYaml()

	lg := config.NewLogger()

	ma := network.NewMatcher(yam.Route.Paths)
	cl := network.NewShadowClient()
	pr := network.NewReverseProxy(
		yam.Route,
		lg,
		yam.MaxBodyBytes,
	)

	di := differ.NewDiffer()

	sh := shadow.NewShadow(lg, cl.GetClient())

	st, err := store.NewStore(ctx, lg, yam.DatabasePath)
	if err != nil {
		lg.Error("failed to create store", "err", err)
		return
	}

	ha := orchestrator.NewOrchestrator(
		lg,
		pr,
		di,
		ma,
		sh,
		st,
		yam.Route,
		yam.ShadowTimeoutMS,
		yam.E2ETimeoutMS,
		yam.MaxBodyBytes,
	)

	srv := network.NewServer(
		yam.Listen,
		ha,
		time.Duration(yam.ReadHeaderTimeoutMS)*time.Millisecond,
		time.Duration(yam.IdleTimeoutMS)*time.Millisecond,
	)

	storeCtx, storeCancel := context.WithCancel(context.Background())
	defer storeCancel()

	go st.Run(storeCtx)
	go func() {
		if sErr := srv.ListenAndServe(); sErr != nil && !errors.Is(sErr, http.ErrServerClosed) {
			lg.Error("server stopped unexpectedly", "err", sErr)
			cancel()
		}
	}()

	<-ctx.Done()

	shutdownCtx, stop := context.WithTimeout(context.Background(), time.Second*10)
	defer stop()

	if qErr := srv.Shutdown(shutdownCtx); qErr != nil {
		lg.Error("failed to shutdown server", "err", qErr)
	}

	ha.Wait()

	pr.CloseIdleConnections()
	cl.CloseIdleConnections()

	storeCancel()
	st.Wait()
	st.Close()
}
