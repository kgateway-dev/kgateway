package admin

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/servers/iosnapshot"
	"net/http"
)

const (
	InputSnapshotEndpoint = "/snapshots/input"
)

func ServerHandlers(history iosnapshot.History) func(mux *http.ServeMux, profiles map[string]string) {
	return func(mux *http.ServeMux, profiles map[string]string) {
		mux.Handle(InputSnapshotEndpoint, iosnapshot.NewInputServer(history))
	}
}
