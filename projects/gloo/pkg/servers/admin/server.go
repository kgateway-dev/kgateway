package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/solo-io/gloo/projects/gloo/pkg/servers/iosnapshot"
	"istio.io/istio/pkg/kube/krt"
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

		// The Edge Snapshot is intended to return a representation of the ApiSnapshot object that the Control Plane
		// manages internally. This is not intended to be consumed by users, but instead be a mechanism to feed this
		// data into future unit tests
		m.HandleFunc("/snapshots/edge", func(w http.ResponseWriter, request *http.Request) {
			response := history.GetEdgeApiSnapshot(ctx)
			respondJson(w, response)
		})

		// The Proxy Snapshot is intended to return a representation of the Proxies within the ApiSnapshot object.
		// Proxies may either be persisted in etcD or in-memory, so this Api provides a single mechansim to access
		// these resources.
		m.HandleFunc("/snapshots/proxies", func(w http.ResponseWriter, r *http.Request) {
			response := history.GetProxySnapshot(ctx)
			respondJson(w, response)
		})

		// The xDS Snapshot is intended to return the full in-memory xDS cache that the Control Plane manages
		// and serves up to running proxies.
		m.HandleFunc("/snapshots/xds", func(w http.ResponseWriter, r *http.Request) {
			response := history.GetXdsSnapshot(ctx)
			respondJson(w, response)
		})
		m.HandleFunc("/snapshots/krt", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, dbg, r)
		})
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
