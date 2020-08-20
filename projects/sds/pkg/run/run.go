package run

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	"github.com/solo-io/gloo/projects/sds/pkg/server"
	"github.com/solo-io/go-utils/contextutils"
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
				contextutils.LoggerFrom(ctx).Infow("received event", zap.Any("event", event))
				sdsServer.UpdateSDSConfig(ctx)
				watchFiles(ctx, watcher, secrets)
			// watch for errors
			case err := <-watcher.Errors:
				contextutils.LoggerFrom(ctx).Warnw("Received error from file watcher", zap.Error(err))
			case <-ctx.Done():
				return
			}
		}
	}()
	watchFiles(ctx, watcher, secrets)

	<-sigs
	cancel()
	select {
	case <-serverStopped:
		return nil
	case <-time.After(3 * time.Second):
		return nil
	}
}

func watchFiles(ctx context.Context, watcher *fsnotify.Watcher, secrets []server.Secret) {
	for _, s := range secrets {
		contextutils.LoggerFrom(ctx).Infow("watcher started", zap.String("sslKeyFile", s.SslKeyFile), zap.String("sshCertFile", s.SslCertFile), zap.String("sslCaFile", s.SslCaFile))
		if err := watcher.Add(s.SslKeyFile); err != nil {
			contextutils.LoggerFrom(ctx).Warn(zap.Error(err))
		}
		if err := watcher.Add(s.SslCertFile); err != nil {
			contextutils.LoggerFrom(ctx).Warn(zap.Error(err))
		}
		if err := watcher.Add(s.SslCaFile); err != nil {
			contextutils.LoggerFrom(ctx).Warn(zap.Error(err))
		}
	}
}
