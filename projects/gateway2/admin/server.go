package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/stats"
	"istio.io/istio/pkg/kube/krt"
)

const (
	AdminPort = 9095
)

func RunAdminServer(ctx context.Context, setupOpts *controller.SetupOpts) error {
	// serverHandlers defines the custom handlers that the Admin Server will support
	serverHandlers := getServerHandlers(ctx, setupOpts.KrtDebugger, setupOpts.Cache)

	stats.StartCancellableStatsServerWithPort(ctx, stats.DefaultStartupOptions(), func(mux *http.ServeMux, profiles map[string]string) {
		// let people know these moved
		profiles[fmt.Sprintf("http://localhost:%d/snapshots/", AdminPort)] = fmt.Sprintf("To see snapshots, port forward to port %d", AdminPort)
	})
	startHandlers(ctx, serverHandlers)

	return nil
}

// getServerHandlers returns the custom handlers for the Admin Server, which will be bound to the http.ServeMux
// These endpoints serve as the basis for an Admin Interface for the Control Plane (https://github.com/solo-io/gloo/issues/6494)
func getServerHandlers(ctx context.Context, dbg *krt.DebugHandler, cache envoycache.SnapshotCache) func(mux *http.ServeMux, profiles map[string]string) {
	return func(m *http.ServeMux, profiles map[string]string) {

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

		// The xDS Snapshot is intended to return the full in-memory xDS cache that the Control Plane manages
		// and serves up to running proxies.
		m.HandleFunc("/snapshots/xds", func(w http.ResponseWriter, r *http.Request) {
			response := getXdsSnapshotDataFromCache(cache)
			writeJSON(w, response, r)
		})
		profiles["/snapshots/xds"] = "XDS Snapshot"

		m.HandleFunc("/snapshots/krt", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, dbg, r)
		})
		profiles["/snapshots/krt"] = "KRT Snapshot"
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

func startHandlers(ctx context.Context, addHandlers ...func(mux *http.ServeMux, profiles map[string]string)) error {
	mux := new(http.ServeMux)
	profileDescriptions := map[string]string{}
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

func index(profileDescriptions map[string]string) func(w http.ResponseWriter, r *http.Request) {
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

func getXdsSnapshotDataFromCache(xdsCache cache.SnapshotCache) SnapshotResponseData {
	cacheKeys := xdsCache.GetStatusKeys()
	cacheEntries := make(map[string]interface{}, len(cacheKeys))

	for _, k := range cacheKeys {
		xdsSnapshot, err := getXdsSnapshot(xdsCache, k)
		if err != nil {
			cacheEntries[k] = err.Error()
		} else {
			cacheEntries[k] = xdsSnapshot
		}
	}

	return completeSnapshotResponse(cacheEntries)
}

func getXdsSnapshot(xdsCache cache.SnapshotCache, k string) (cache cache.ResourceSnapshot, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = eris.New(fmt.Sprintf("panic occurred while getting xds snapshot: %v", r))
		}
	}()
	return xdsCache.GetSnapshot(k)
}
