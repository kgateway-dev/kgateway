package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/kgateway-dev/kgateway/projects/gateway2/controller"
	"github.com/solo-io/go-utils/contextutils"
	"istio.io/istio/pkg/kube/krt"
)

const (
	AdminPort = 9097
)

func RunAdminServer(ctx context.Context, setupOpts *controller.SetupOpts) error {
	// serverHandlers defines the custom handlers that the Admin Server will support
	serverHandlers := getServerHandlers(ctx, setupOpts.KrtDebugger, setupOpts.Cache)

	// initialize the atomic log level
	if envLogLevel := os.Getenv(contextutils.LogLevelEnvName); envLogLevel != "" {
		contextutils.SetLogLevelFromString(envLogLevel)
	}

	startHandlers(ctx, serverHandlers)

	return nil
}

// use a function for the profile descriptions so that every time the admin page is displayed, it can show
// up-to-date info in the description (e.g. the current log level)
type dynamicProfileDescription func() string

// getServerHandlers returns the custom handlers for the Admin Server, which will be bound to the http.ServeMux
// These endpoints serve as the basis for an Admin Interface for the Control Plane (https://github.com/kgateway-dev/kgateway/issues/6494)
func getServerHandlers(ctx context.Context, dbg *krt.DebugHandler, cache envoycache.SnapshotCache) func(mux *http.ServeMux, profiles map[string]dynamicProfileDescription) {
	return func(m *http.ServeMux, profiles map[string]dynamicProfileDescription) {

		/*
			// The Input Snapshot is intended to return a list of resources that are persisted in the Kubernetes DB, etcD
			m.HandleFunc("/snapshots/input", func(w http.ResponseWriter, request *http.Request) {
				response := history.GetInputSnapshot(ctx)
				respondJson(w, response)
			})
			profiles["/snapshots/input"] = "Input Snapshot"

			// The Proxy Snapshot is intended to return a representation of the Proxies within the ApiSnapshot object.
			// Proxies may either be persisted in etcD or in-memory, so this Api provides a single mechansim to access
			// these resources.
			m.HandleFunc("/snapshots/proxies", func(w http.ResponseWriter, r *http.Request) {
				response := history.GetProxySnapshot(ctx)
				respondJson(w, response)
			})
			profiles["/snapshots/proxies"] = "Proxy Snapshot"
		*/

		addXdsSnapshotHandler("/snapshots/xds", m, profiles, cache)

		addKrtSnapshotHandler("/snapshots/krt", m, profiles, dbg)

		addLoggingHandler("/logging", m, profiles)

		addPprofHandler("/debug/pprof/", m, profiles)
	}
}

func respondJson(w http.ResponseWriter, response SnapshotResponseData) {
	w.Header().Set("Content-Type", getContentType("json"))

	_, _ = fmt.Fprintf(w, "%+v", response.MarshalJSONString())
}

func getContentType(format string) string {
	switch format {
	case "", "json", "json_compact":
		return "application/json"
	case "yaml":
		return "text/x-yaml"
	default:
		return "application/json"
	}
}

// writeJSON writes a json payload, handling content type, marshaling, and errors
func writeJSON(w http.ResponseWriter, obj any, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	pretty := req.URL.Query().Has("pretty")
	indent := ""
	if pretty {
		indent = "    "
	}

	var b []byte
	var err error
	if pretty {
		b, err = json.MarshalIndent(obj, "", indent)
	} else {
		b, err = json.Marshal(obj)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	_, err = w.Write(b)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func startHandlers(ctx context.Context, addHandlers ...func(mux *http.ServeMux, profiles map[string]dynamicProfileDescription)) error {
	mux := new(http.ServeMux)
	profileDescriptions := map[string]dynamicProfileDescription{}
	for _, addHandler := range addHandlers {
		addHandler(mux, profileDescriptions)
	}
	idx := index(profileDescriptions)
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
			if err != nil {
				contextutils.LoggerFrom(ctx).Warnf("Admin server shutdown returned error: %v", err)
			}
		}
	}()
	return nil
}

func index(profileDescriptions map[string]dynamicProfileDescription) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		type profile struct {
			Name string
			Href string
			Desc string
		}
		var profiles []profile
		for href, descFunc := range profileDescriptions {
			profiles = append(profiles, profile{
				Name: href,
				Href: href,
				Desc: descFunc(),
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
