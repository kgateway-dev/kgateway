package utils

import (
	"fmt"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// RouteConfigName cannot always be relied on to get the route config name for any listener because
// the pattern is different for hybrid listeners (see MatchedRouteConfigName below)
func RouteConfigName(listener *v1.Listener) string {
	return listener.GetName() + "-routes"
}

func MatchedRouteConfigName(listener *v1.Listener, matcher *v1.Matcher) string {
	hybridListener := listener.GetHybridListener()
	if hybridListener == nil {
		return RouteConfigName(listener)
	}

	for i, mg := range hybridListener.GetMatchedListeners() {
		if mg.GetMatcher().Equal(matcher) {
			return fmt.Sprintf("%s-%d", RouteConfigName(listener), i)
		}
	}

	return fmt.Sprintf("%s-%s", RouteConfigName(listener), matcher.String())
}
