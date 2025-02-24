package probes

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/solo-io/go-utils/contextutils"
)

type ServerParams struct {
	Port         int
	Path         string
	ResponseCode int
	ResponseBody string
}

// NewServerParams creates gloo's default probe server parameters
func NewServerParams() ServerParams {
	return ServerParams{
		Port:         8765,
		Path:         "/healthz",
		ResponseCode: http.StatusOK,
		ResponseBody: "OK\n",
	}
}

// StartServer accepts a port and opens a simple http server
// which will respond to requests at the configured port and path
func StartServer(ctx context.Context, params ServerParams) {
	var server *http.Server

	// make sure we don't blow up on a bad call with some sane defaults
	if !strings.HasPrefix(params.Path, "/") {
		params.Path = "/" + params.Path
	}
	if params.Port == 0 {
		params.Port = 8765
	}
	if params.ResponseCode == 0 {
		params.ResponseCode = http.StatusOK
	}

	// Run the server in a goroutine
	go func() {
		mux := new(http.ServeMux)
		mux.HandleFunc(params.Path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(params.ResponseCode)
			w.Write([]byte(params.ResponseBody))
		})
		server = &http.Server{
			Addr:    fmt.Sprintf(":%d", params.Port),
			Handler: mux,
		}
		contextutils.LoggerFrom(ctx).Infof("probe server starting at %s listening for %s", server.Addr, params.Path)
		err := server.ListenAndServe()
		if err == http.ErrServerClosed {
			contextutils.LoggerFrom(ctx).Info("probe server closed")
		} else {
			contextutils.LoggerFrom(ctx).Warnf("probe server closed with unexpected error: %v", err)
		}
	}()

	// Run a separate goroutine to handle the server shutdown when the context is cancelled
	go func() {
		<-ctx.Done()
		if server != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				contextutils.LoggerFrom(shutdownCtx).Warnf("probe server shutdown returned error: %v", err)
			}
		}
	}()
}
