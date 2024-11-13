package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/solo-io/gloo/projects/gloo/pkg/servers/iosnapshot"
	"github.com/solo-io/go-utils/contextutils"
	"istio.io/istio/pkg/kube/krt"
)

const (
	AdminPort = 9095
)

// ServerHandlers returns the custom handlers for the Admin Server, which will be bound to the http.ServeMux
// These endpoints serve as the basis for an Admin Interface for the Control Plane (https://github.com/solo-io/gloo/issues/6494)
func ServerHandlers(ctx context.Context, history iosnapshot.History, dbg *krt.DebugHandler) func(mux *http.ServeMux, profiles map[string]string) {
	return func(m *http.ServeMux, profiles map[string]string) {

		// The Input Snapshot is intended to return a list of resources that are persisted in the Kubernetes DB, etcD
		m.HandleFunc("/snapshots/input", func(w http.ResponseWriter, request *http.Request) {
			response := history.GetInputSnapshot(ctx)
			respondJson(w, response)
		})
		profiles["/snapshots/input"] = "Input Snapshot"

		// The Edge Snapshot is intended to return a representation of the ApiSnapshot object that the Control Plane
		// manages internally. This is not intended to be consumed by users, but instead be a mechanism to feed this
		// data into future unit tests
		m.HandleFunc("/snapshots/edge", func(w http.ResponseWriter, request *http.Request) {
			response := history.GetEdgeApiSnapshot(ctx)
			respondJson(w, response)
		})
		profiles["/snapshots/edge"] = "Edge Snapshot"

		// The Proxy Snapshot is intended to return a representation of the Proxies within the ApiSnapshot object.
		// Proxies may either be persisted in etcD or in-memory, so this Api provides a single mechansim to access
		// these resources.
		m.HandleFunc("/snapshots/proxies", func(w http.ResponseWriter, r *http.Request) {
			response := history.GetProxySnapshot(ctx)
			respondJson(w, response)
		})
		profiles["/snapshots/proxies"] = "Proxy Snapshot"

		// The xDS Snapshot is intended to return the full in-memory xDS cache that the Control Plane manages
		// and serves up to running proxies.
		m.HandleFunc("/snapshots/xds", func(w http.ResponseWriter, r *http.Request) {
			response := history.GetXdsSnapshot(ctx)
			respondJson(w, response)
		})
		profiles["/snapshots/xds"] = "XDS Snapshot"

		m.HandleFunc("/snapshots/krt", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, dbg, r)
		})
		profiles["/snapshots/krt"] = "KRT Snapshot"
	}
}

func respondJson(w http.ResponseWriter, response iosnapshot.SnapshotResponseData) {
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

func StartHandlers(ctx context.Context, addHandlers ...func(mux *http.ServeMux, profiles map[string]string)) error {
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
