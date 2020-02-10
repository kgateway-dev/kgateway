package run

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/solo-io/gloo/projects/sds/pkg/server"
	"github.com/solo-io/go-utils/contextutils"
	v1 "k8s.io/api/core/v1"
)

const (
	secretDir   = "/etc/envoy/ssl/"
	sslKeyFile  = secretDir + v1.TLSPrivateKeyKey        // tls.key
	sslCertFile = secretDir + v1.TLSCertKey              //tls.crt
	sslCaFile   = secretDir + v1.ServiceAccountRootCAKey //ca.crt
	sdsServerAddress = "0.0.0.0:8236"
)

func Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	// Set up the gRPC server
	grpcServer, snapshotCache := server.SetupEnvoySDS()

	// Run the gRPC Server
	err := server.RunSDSServer(ctx, grpcServer, sdsServerAddress) // runs the grpc server in internal goroutines
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
				contextutils.LoggerFrom(ctx).Info("received event: \n", event)
				server.UpdateSDSConfig(ctx, sslKeyFile, sslCertFile, sslCaFile, snapshotCache)
			// watch for errors
			case err := <-watcher.Errors:
				contextutils.LoggerFrom(ctx).Warn("Received error: \n", err)
			case <-ctx.Done():
				return
			}
		}
	}()
	if err := watcher.Add(sslCertFile); err != nil {
		return err
	}
	if err := watcher.Add(sslKeyFile); err != nil {
		return err
	}
	if err := watcher.Add(sslCaFile); err != nil {
		return err
	}

	<-sigs
	cancel()
	return nil
}
