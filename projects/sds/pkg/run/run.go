package run

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	sds_server "github.com/solo-io/gloo/projects/sds/pkg/server"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/fsnotify/fsnotify"
	"github.com/solo-io/go-utils/contextutils"
)

var (
	grpcOptions = []grpc.ServerOption{grpc.MaxConcurrentStreams(10000)}
)

func Run(
	rootCtx context.Context,
	sslKeyFile, sslCertFile,
	sslCaFile,
	sdsServerAddress string,
	sdsServerFactories []sds_server.EnvoySdsServerFactory,
) error {
	ctx, cancel := context.WithCancel(rootCtx)

	// Initialize the Server
	grpcServer := grpc.NewServer(grpcOptions...)

	// Initialize the Services
	var sdsServers []sds_server.EnvoySdsServer
	for _, factory := range sdsServerFactories {
		sdsServers = append(sdsServers, factory(ctx, grpcServer))
	}

	// Run the gRPC Server
	serverStopped, err := runSDSServer(ctx, grpcServer, sdsServerAddress) // runs the grpc server in internal goroutines
	if err != nil {
		return err
	}

	// Initialize the SDS config
	for _, srv := range sdsServers {
		if err = srv.UpdateSDSConfig(ctx, sslKeyFile, sslCertFile, sslCaFile); err != nil {
			return err
		}
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
				for _, srv := range sdsServers {
					srv.UpdateSDSConfig(ctx, sslKeyFile, sslCertFile, sslCaFile)
				}
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

func runSDSServer(ctx context.Context, grpcServer *grpc.Server, serverAddress string) (<-chan struct{}, error) {
	lis, err := net.Listen("tcp", serverAddress)
	if err != nil {
		return nil, err
	}
	contextutils.LoggerFrom(ctx).Infof("sds server listening on %s", serverAddress)
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			contextutils.LoggerFrom(ctx).Fatalw("fatal error in gRPC server", zap.String("address", serverAddress), zap.Error(err))
		}
	}()
	serverStopped := make(chan struct{})
	go func() {
		<-ctx.Done()
		contextutils.LoggerFrom(ctx).Infof("stopping sds server on %s\n", serverAddress)
		grpcServer.GracefulStop()
		serverStopped <- struct{}{}
	}()
	return serverStopped, nil
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
