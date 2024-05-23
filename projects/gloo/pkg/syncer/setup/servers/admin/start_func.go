package admin

import (
	"context"
	"net/http"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup/servers/iosnapshot"
	"github.com/solo-io/go-utils/stats"
)

const (
	InputSnapshotEndpoint = "/snapshots/input"
)

// StartFunc returns the setup.StartFunc for the Admin Server
// The Admin Server is the groundwork for an Administration Interface, similar to the of Envoy
// https://github.com/solo-io/gloo/issues/6494
func StartFunc(history iosnapshot.History) setup.StartFunc {

	// serverHandlers defines the custom handlers that the Admin Server will support
	var serverHandlers = func(mux *http.ServeMux, profiles map[string]string) {
		mux.Handle(InputSnapshotEndpoint, iosnapshot.NewInputServer(history))
	}

	return func(ctx context.Context, opts bootstrap.Opts, extensions setup.Extensions) error {
		// The Stats Server is used as the running server for our admin endpoints
		//
		// NOTE: There is a slight difference in how we run this server -vs- how we used to run it
		// In the past, we would start the server once, at the beginning of the running container
		// Now, we start a new server each time we invoke a StartFunc.
		stats.StartCancellableStatsServerWithPort(ctx, stats.DefaultStartupOptions(), serverHandlers)

		return nil
	}
}
