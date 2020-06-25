package run

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/fsnotify/fsnotify"
	"github.com/solo-io/gloo/projects/sds/pkg/server"
	"github.com/solo-io/go-utils/contextutils"
)

func Run(ctx context.Context, sslKeyFile, sslCertFile, sslCaFile, sdsServerAddress string) error {
	ctx, cancel := context.WithCancel(ctx)

	// Set up the gRPC server
	grpcServer, snapshotCache := server.SetupEnvoySDS()

	// Run the gRPC Server
	serverStopped, err := server.RunSDSServer(ctx, grpcServer, sdsServerAddress) // runs the grpc server in internal goroutines
	if err != nil {
		return err
	}

	// Initialize the SDS config
	err = server.UpdateSDSConfig(ctx, sslKeyFile, sslCertFile, sslCaFile, snapshotCache)
	if err != nil {
		return err
	}

	// create a new file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
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
				server.UpdateSDSConfig(ctx, sslKeyFile, sslCertFile, sslCaFile, snapshotCache)
				watchFiles(ctx, watcher, sslKeyFile, sslCertFile, sslCaFile)
			// watch for errors
			case err := <-watcher.Errors:
				contextutils.LoggerFrom(ctx).Warnw("Received error from file watcher", zap.Error(err))
			case <-ctx.Done():
				return
			}
		}
	}()
	watchFiles(ctx, watcher, sslKeyFile, sslCertFile, sslCaFile)

	<-sigs
	cancel()
	select {
	case <-serverStopped:
		return nil
	case <-time.After(3 * time.Second):
		return nil
	}
}

func watchFiles(ctx context.Context, watcher *fsnotify.Watcher, sslKeyFile string, sslCertFile string, sslCaFile string) {
	if err := watcher.Add(sslKeyFile); err != nil {
		contextutils.LoggerFrom(ctx).Warn(zap.Error(err))
	}
	if err := watcher.Add(sslCertFile); err != nil {
		contextutils.LoggerFrom(ctx).Warn(zap.Error(err))
	}
	if err := watcher.Add(sslCaFile); err != nil {
		contextutils.LoggerFrom(ctx).Warn(zap.Error(err))
	}
}
