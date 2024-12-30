package krtcollections

import "github.com/solo-io/gloo/projects/gateway2/ir"

func (h *RoutesIndex) TEST() []ir.HttpRouteIR {
	return h.httpRoutes.List()
}
