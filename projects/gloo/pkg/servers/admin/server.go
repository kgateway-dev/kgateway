package admin

import (
	"context"
	"fmt"
	"net/http"

	"github.com/solo-io/gloo/projects/gloo/pkg/servers/iosnapshot"
)

// ServerHandlers returns the custom handlers for the Admin Server, which will be bound to the http.ServeMux
// These endpoints serve as the basis for an Admin Interface for the Control Plane (https://github.com/solo-io/gloo/issues/6494)
func ServerHandlers(ctx context.Context, history iosnapshot.History) func(mux *http.ServeMux, profiles map[string]string) {
	return func(m *http.ServeMux, profiles map[string]string) {
		m.HandleFunc("/snapshots/input", func(w http.ResponseWriter, request *http.Request) {
			response := history.GetInputSnapshot(ctx)
			respondJson(w, response)
		})

		m.HandleFunc("/snapshots/edge", func(w http.ResponseWriter, request *http.Request) {
			response := history.GetEdgeApiSnapshot(ctx)
			respondJson(w, response)
		})

		m.HandleFunc("/snapshots/proxies", func(w http.ResponseWriter, r *http.Request) {
			response := history.GetProxySnapshot(ctx)
			respondJson(w, response)
		})

		m.HandleFunc("/snapshots/xds", func(w http.ResponseWriter, r *http.Request) {
			response := history.GetXdsSnapshot(ctx)
			respondJson(w, response)
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
