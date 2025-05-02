package run

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/kgateway-dev/kgateway/v2/internal/sds/pkg/server"
)

func Run(ctx context.Context, secrets []server.Secret, sdsClient, sdsServerAddress string) error {
	ctx, cancel := context.WithCancel(ctx)

	// Set up the gRPC server
	sdsServer := server.SetupEnvoySDS(secrets, sdsClient, sdsServerAddress)
	// Run the gRPC Server
	serverStopped, err := sdsServer.Run(ctx) // runs the grpc server in internal goroutines
	if err != nil {
		cancel()
		return err
	}

	// Initialize the SDS config
	err = sdsServer.UpdateSDSConfig(ctx)
	if err != nil {
		cancel()
		return err
	}

	// create a new file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		cancel()
		return err
	}
	defer watcher.Close()

	// Wire in signal handling
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			select {
			// watch for events
			case event := <-watcher.Events:
				slog.Info("received event", slog.Any("event", event))
				sdsServer.UpdateSDSConfig(ctx)
				watchFiles(watcher, secrets)
			// watch for errors
			case err := <-watcher.Errors:
				slog.Warn("Received error from file watcher", slog.Any("error", err))
			case <-ctx.Done():
				return
			}
		}
	}()
	watchFiles(watcher, secrets)

	<-sigs
	cancel()
	select {
	case <-serverStopped:
		return nil
	case <-time.After(3 * time.Second):
		return nil
	}
}

func watchFiles(watcher *fsnotify.Watcher, secrets []server.Secret) {
	for _, s := range secrets {
		slog.Info("watcher started", slog.String("sslKeyFile", s.SslKeyFile), slog.String("sshCertFile", s.SslCertFile), slog.String("sslCaFile", s.SslCaFile))
		if err := watcher.Add(s.SslKeyFile); err != nil {
			slog.Warn("failed to add watch for key file", slog.Any("error", err), slog.String("file", s.SslKeyFile))
		}
		if err := watcher.Add(s.SslCertFile); err != nil {
			slog.Warn("failed to add watch for cert file", slog.Any("error", err), slog.String("file", s.SslCertFile))
		}
		if err := watcher.Add(s.SslCaFile); err != nil {
			slog.Warn("failed to add watch for ca file", slog.Any("error", err), slog.String("file", s.SslCaFile))
		}
	}
}
