package admin

import (
	"fmt"
	"net/http"

	"github.com/solo-io/gloo/projects/gloo/pkg/servers/iosnapshot"
)

// ServerHandlers returns the custom handlers for the Admin Server, which will be bound to the http.ServeMux
// These endpoints serve as the basis for an Admin Interface for the Control Plane (https://github.com/solo-io/gloo/issues/6494)
func ServerHandlers(history iosnapshot.History) func(mux *http.ServeMux, profiles map[string]string) {
	return func(m *http.ServeMux, profiles map[string]string) {
		m.HandleFunc("/snapshots/input", func(w http.ResponseWriter, request *http.Request) {
			inputSnap, err := history.GetInputSnapshot()
			if err != nil {
				respondError(w, err)
				return
			}

			respondJson(w, inputSnap)
		})

		m.HandleFunc("/snapshots/proxies", func(w http.ResponseWriter, r *http.Request) {
			proxySnap, err := history.GetProxySnapshot()
			if err != nil {
				respondError(w, err)
				return
			}

			respondJson(w, proxySnap)
		})

		m.HandleFunc("/snapshots/xds", func(w http.ResponseWriter, r *http.Request) {
			xdsEntries, err := history.GetXdsSnapshot()
			if err != nil {
				respondError(w, err)
				return
			}

			respondJson(w, xdsEntries)
		})
	}
}

func respondJson(w http.ResponseWriter, response []byte) {
	w.Header().Set("Content-Type", getContentType("json"))

	_, _ = fmt.Fprintf(w, "%+v", string(response))
}

func respondError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
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
