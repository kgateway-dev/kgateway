package run

import (
	"context"
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
				logger.Info("received event", "event", event)
				sdsServer.UpdateSDSConfig(ctx)
				watchFiles(watcher, secrets)
			// watch for errors
			case err := <-watcher.Errors:
				logger.Warn("received error from file watcher", "error", err)
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
		logger.Info("watcher started", "key_file", s.SslKeyFile, "cert_file", s.SslCertFile, "ca_file", s.SslCaFile)
		if err := watcher.Add(s.SslKeyFile); err != nil {
			logger.Warn("failed to add watch for key file", "error", err, "file", s.SslKeyFile)
		}
		if err := watcher.Add(s.SslCertFile); err != nil {
			logger.Warn("failed to add watch for cert file", "error", err, "file", s.SslCertFile)
		}
		if err := watcher.Add(s.SslCaFile); err != nil {
			logger.Warn("failed to add watch for ca file", "error", err, "file", s.SslCaFile)
		}
	}
}
