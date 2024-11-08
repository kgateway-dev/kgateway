package setup

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"

	"github.com/solo-io/gloo/projects/gloo/pkg/servers/admin"
	"github.com/solo-io/gloo/projects/gloo/pkg/servers/iosnapshot"
	"github.com/solo-io/go-utils/stats"
	"istio.io/istio/pkg/kube/krt"

	"golang.org/x/sync/errgroup"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
)

const (
	AdminPort = 9095
)

// StartFunc represents a function that will be called with the initialized bootstrap.Opts
// and Extensions. This is invoked each time the setup_syncer is executed
// (which runs whenever the Setting CR is modified)
type StartFunc func(ctx context.Context, opts bootstrap.Opts, extensions Extensions) error

// ExecuteAsynchronousStartFuncs accepts a collection of StartFunc inputs, and executes them within an Error Group
func ExecuteAsynchronousStartFuncs(
	ctx context.Context,
	opts bootstrap.Opts,
	extensions Extensions,
	startFuncs map[string]StartFunc,
	errorGroup *errgroup.Group,
) {
	for name, start := range startFuncs {
		startFn := start // pike
		namedCtx := contextutils.WithLogger(ctx, name)

		errorGroup.Go(
			func() error {
				contextutils.LoggerFrom(namedCtx).Infof("starting %s goroutine", name)
				err := startFn(namedCtx, opts, extensions)
				if err != nil {
					contextutils.LoggerFrom(namedCtx).Errorf("%s goroutine failed: %v", name, err)
				}
				return err
			},
		)
	}

	contextutils.LoggerFrom(ctx).Debug("main goroutines successfully started")
}

// AdminServerStartFunc returns the setup.StartFunc for the Admin Server
// The Admin Server is the groundwork for an Administration Interface, similar to that of Envoy
// https://github.com/solo-io/gloo/issues/6494
// The endpoints that are available on this server are split between two places:
//  1. The default endpoints are defined by our stats server: https://github.com/solo-io/go-utils/blob/8eda16b9878d71673e6a3a9756f6088160f75468/stats/stats.go#L79
//  2. Custom endpoints are defined by our admin server handler in `gloo/pkg/servers/admin`
func AdminServerStartFunc(history iosnapshot.History, dbg *krt.DebugHandler) StartFunc {
	return func(ctx context.Context, opts bootstrap.Opts, extensions Extensions) error {
		// serverHandlers defines the custom handlers that the Admin Server will support
		serverHandlers := admin.ServerHandlers(ctx, history, dbg)

		// The Stats Server is used as the running server for our admin endpoints
		//
		// NOTE: There is a slight difference in how we run this server -vs- how we used to run it
		// In the past, we would start the server once, at the beginning of the running container
		// Now, we start a new server each time we invoke a StartFunc.
		if serverAdminHandlersWithStats() {
			stats.StartCancellableStatsServerWithPort(ctx, stats.DefaultStartupOptions(), serverHandlers)
		} else {
			stats.StartCancellableStatsServerWithPort(ctx, stats.DefaultStartupOptions(), func(mux *http.ServeMux, profiles map[string]string) {
				// let people know these moved
				profiles[fmt.Sprintf("http://localhost:%d/snapshots/", AdminPort)] = fmt.Sprintf("To see snapshots, port forward to port %d", AdminPort)
			})
			startHandlers(ctx, serverHandlers)
		}

		return nil
	}
}

func startHandlers(ctx context.Context, addHandlers ...func(mux *http.ServeMux, profiles map[string]string)) error {
	mux := new(http.ServeMux)
	profileDescriptions := map[string]string{}
	for _, addHandler := range addHandlers {
		addHandler(mux, profileDescriptions)
	}
	idx := Index(profileDescriptions)
	mux.HandleFunc("/", idx)
	mux.HandleFunc("/snapshots/", idx)
	server := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", AdminPort),
		Handler: mux,
	}
	contextutils.LoggerFrom(ctx).Infof("Admin server starting at %s", server.Addr)
	go func() {
		err := server.ListenAndServe()
		if err == http.ErrServerClosed {
			contextutils.LoggerFrom(ctx).Infof("Admin server closed")
		} else {
			contextutils.LoggerFrom(ctx).Warnf("Admin server closed with unexpected error: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		if server != nil {
			err := server.Close()
			contextutils.LoggerFrom(ctx).Warnf("Admin server shutdown returned error: %v", err)
		}
	}()
	return nil
}

func serverAdminHandlersWithStats() bool {
	env := os.Getenv("ADMIN_HANDLERS_WITH_STATS")
	return env == "true"
}

func Index(profileDescriptions map[string]string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		type profile struct {
			Name string
			Href string
			Desc string
		}
		var profiles []profile
		for href, desc := range profileDescriptions {
			profiles = append(profiles, profile{
				Name: href,
				Href: href,
				Desc: desc,
			})
		}

		sort.Slice(profiles, func(i, j int) bool {
			return profiles[i].Name < profiles[j].Name
		})

		// Adding other profiles exposed from within this package
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "<h1>Admin Server</h1>\n")
		for _, p := range profiles {
			fmt.Fprintf(&buf, "<h2><a href=\"%s\"}>%s</a></h2><p>%s</p>\n", p.Name, p.Name, p.Desc)

		}
		w.Write(buf.Bytes())
	}
}
