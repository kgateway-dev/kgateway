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

// MatchedRouteConfigName returns a unique RouteConfiguration name
// This name is commonly used for 2 purposes:
//  1. to associate the RouteConfigurationName between RDS and the HttpConnectionManager NetworkFilter
//  2. To provide a consistent key function for a map of ListenerReports
func MatchedRouteConfigName(listener *v1.Listener, matcher *v1.Matcher) string {
	namePrefix := RouteConfigName(listener)

	if matcher == nil {
		return namePrefix
	}
	nameSuffix, _ := matcher.Hash(nil)
	return fmt.Sprintf("%s-%d", namePrefix, nameSuffix)
}
