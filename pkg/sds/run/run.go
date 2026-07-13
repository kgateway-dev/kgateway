package run

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/kgateway-dev/kgateway/v2/pkg/sds/server"
)

// sdsUpdateDebounce is the quiet period after the last fsnotify event before
// reloading certs from disk, so writers can finish updating related files.
const sdsUpdateDebounce = 500 * time.Millisecond

func Run(ctx context.Context, secrets []server.Secret, sdsClient, sdsServerAddress string, logger *slog.Logger) error {
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
	defer signal.Stop(sigs)

	// Add the watches before the goroutine starts so re-adding watches does not
	// race with the initial watcher setup.
	watchFiles(watcher, secrets, logger)

	go func() {
		runWatcherLoop(ctx, watcher, logger, func(ctx context.Context) {
			if err := sdsServer.UpdateSDSConfig(ctx); err != nil {
				logger.Warn("failed to update SDS config after cert file change", "error", err)
			}
			if ctx.Err() != nil {
				return
			}
			watchFiles(watcher, secrets, logger)
		})
	}()

	select {
	case <-sigs:
	case <-ctx.Done():
	}
	cancel()
	select {
	case <-serverStopped:
		return nil
	case <-time.After(3 * time.Second):
		return nil
	}
}

func runWatcherLoop(ctx context.Context, watcher *fsnotify.Watcher, logger *slog.Logger, onDebouncedUpdate func(context.Context)) {
	debounceTimer := time.NewTimer(sdsUpdateDebounce)
	stopAndDrainTimer(debounceTimer)
	defer debounceTimer.Stop()

	var pendingUpdate bool
	for {
		select {
		case event := <-watcher.Events:
			logger.Info("received event", "event", event)
			pendingUpdate = true
			stopAndDrainTimer(debounceTimer)
			debounceTimer.Reset(sdsUpdateDebounce)
		case err := <-watcher.Errors:
			logger.Warn("received error from file watcher", "error", err)
		case <-debounceTimer.C:
			if !pendingUpdate {
				continue
			}
			pendingUpdate = false
			if ctx.Err() != nil {
				return
			}
			onDebouncedUpdate(ctx)
		case <-ctx.Done():
			stopAndDrainTimer(debounceTimer)
			return
		}
	}
}

func stopAndDrainTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

func watchFiles(watcher *fsnotify.Watcher, secrets []server.Secret, logger *slog.Logger) {
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
