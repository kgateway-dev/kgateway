package translator

import v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

func routeConfigName(listener *v1.Listener) string {
	return listener.GetName() + "-routes"
}
