package probes

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/solo-io/go-utils/contextutils"
)

func StartLivenessProbeServer(ctx context.Context) {
	var server *http.Server

	// Run the server in a goroutine
	go func() {
		mux := new(http.ServeMux)
		mux.HandleFunc("/healthz", okHandler)
		server = &http.Server{
			Addr:    fmt.Sprintf(":%d", 8765),
			Handler: mux,
		}
		contextutils.LoggerFrom(ctx).Infof("healthz server starting at %s", server.Addr)
		err := server.ListenAndServe()
		if err == http.ErrServerClosed {
			contextutils.LoggerFrom(ctx).Info("healthz server closed")
		} else {
			contextutils.LoggerFrom(ctx).Warnf("healthz server closed with unexpected error: %v", err)
		}
	}()

	// Run a separate goroutine to handle the server shutdown when the context is cancelled
	go func() {
		<-ctx.Done()
		if server != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				contextutils.LoggerFrom(shutdownCtx).Warnf("healthz server shutdown returned error: %v", err)
			}
		}
	}()
}

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK\n")
}
