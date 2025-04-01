package convert

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gatewayapi/convert/domain"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (g *GatewayAPIOutput) PostProcess(opts *Options) error {

	// complete delegation
	if err := g.finishDelegation(); err != nil {
		return err
	}

	if opts.CombineRouteOptions {
		g.combineRouteOptions()
	}
	if opts.IncludeUnknownResources {
		g.gatewayAPICache.YamlObjects = g.edgeCache.YamlObjects
	}
	return nil
}
func (g *GatewayAPIOutput) finishDelegation() error {

	// for all edge routetables we need to go and update labels on the httproutes to support delegation
	updatedHTTPRoutes := map[string]*domain.HTTPRouteWrapper{}
	for _, rtt := range g.edgeCache.RouteTables {
		routesToUpdate := g.processRouteForDelegation(rtt.Spec.Routes)

		for _, r := range routesToUpdate {
			// check to see if we already matched on this httproute
			updatedHTTPRoute, found := updatedHTTPRoutes[domain.NameNamespaceIndex(r.Name, r.Namespace)]

			if found {
				delegateValue := updatedHTTPRoute.Labels["delegation.gateway.solo.io/label"]
				if delegateValue != r.Labels["delegation.gateway.solo.io/label"] {
					g.AddErrorFromWrapper(ERROR_TYPE_NOT_SUPPORTED, rtt, "HTTPRoute already selected by delegation label delegation.gateway.solo.io/label: %s but also selected by delegation.gateway.solo.io/label: %s", r.Labels["delegation.gateway.solo.io/label"], delegateValue)
					continue
				}
			}
			updatedHTTPRoutes[domain.NameNamespaceIndex(r.Name, r.Namespace)] = r
		}
	}
	for _, vs := range g.edgeCache.VirtualServices {
		routesToUpdate := g.processRouteForDelegation(vs.Spec.VirtualHost.Routes)

		for _, r := range routesToUpdate {
			// check to see if we already matched on this httproute
			updatedHTTPRoute, found := updatedHTTPRoutes[domain.NameNamespaceIndex(r.Name, r.Namespace)]

			if found {
				delegateValue := updatedHTTPRoute.Labels["delegation.gateway.solo.io/label"]
				if delegateValue != r.Labels["delegation.gateway.solo.io/label"] {
					g.AddErrorFromWrapper(ERROR_TYPE_NOT_SUPPORTED, r, "HTTPRoute already selected by delegation label delegation.gateway.solo.io/label: %s but also selected by delegation.gateway.solo.io/label: %s", r.Labels["delegation.gateway.solo.io/label"], delegateValue)
					continue
				}
			}
			updatedHTTPRoutes[domain.NameNamespaceIndex(r.Name, r.Namespace)] = r
		}

	}
	for name, route := range updatedHTTPRoutes {
		g.gatewayAPICache.HTTPRoutes[name] = route
	}

	return nil
}

func (g *GatewayAPIOutput) processRouteForDelegation(routes []*v1.Route) []*domain.HTTPRouteWrapper {
	var routesToUpdate []*domain.HTTPRouteWrapper
	for _, rt := range routes {
		if rt.GetDelegateAction() != nil && rt.GetDelegateAction().GetSelector() != nil {
			selector := rt.GetDelegateAction().GetSelector()
			// go find the RouteTabels by this selector
			namespaces := selector.GetNamespaces()
			if namespaces != nil || len(selector.GetNamespaces()) == 0 {
				// default namespace is gloo-system
				namespaces = []string{"gloo-system"}
			}

			// get all http routes
			for _, httpRoute := range g.gatewayAPICache.HTTPRoutes {
				value, matches := routeMatchSelector(httpRoute, selector)
				if matches {

					if httpRoute.Labels == nil {
						httpRoute.Labels = map[string]string{}
					}
					// this value should match the backend ref name
					//   # Delegate to routes with the label delegation.gateway.solo.io/label:foobar
					//   - group: delegation.gateway.solo.io
					//     kind: label # no other value is allowed
					//     name: single.example.com # label value
					//     namespace: httpbin # defaults to parent's namespace if unset
					httpRoute.Labels["delegation.gateway.solo.io/label"] = value
					routesToUpdate = append(routesToUpdate, httpRoute)
				}
			}
		}
	}
	return routesToUpdate
}

func routeMatchSelector(route *domain.HTTPRouteWrapper, selector *v1.RouteTableSelector) (string, bool) {

	//check namespace first
	if namespaceMatch(route.Namespace, selector.Namespaces) {
		// check to see if any of the labels match the selector
		for k, v := range selector.Labels {
			// see if the route has the label key in the selector
			value, match := route.Labels[k]
			if match {
				// check to see if the values are the same
				if v == value {
					return v, true
				}
			}
		}
	}

	return "", false
}

func namespaceMatch(namespace string, namespaces []string) bool {
	for _, ns := range namespaces {
		if ns == "*" {
			return true
		}
		if ns == namespace {
			return true
		}
	}
	return false
}
func (g *GatewayAPIOutput) combineRouteOptions() {
	var routeOptionKeys []string
	for key, _ := range g.gatewayAPICache.RouteOptions {
		routeOptionKeys = append(routeOptionKeys, key)
	}
	duplicates := map[string][]string{}

	for _, primaryKey := range routeOptionKeys {
		for _, secondaryKey := range routeOptionKeys {
			if primaryKey == secondaryKey {
				// skip if its the same primaryKey
				continue
			}
			ro, found1 := g.gatewayAPICache.RouteOptions[primaryKey]
			if !found1 {
				// this primary primaryKey has already been removed
				//fmt.Printf("primary key %s not found\n", primaryKey)
				break
			}
			ro2, found2 := g.gatewayAPICache.RouteOptions[secondaryKey]
			if !found2 {
				// move on to the next secondaryKey
				continue
			}

			if proto.Equal(&ro.Spec, &ro2.Spec) {
				duplicates[primaryKey] = append(duplicates[primaryKey], secondaryKey)
				//fmt.Printf("Route Option %s matches %s\n", primaryKey, secondaryKey)
				// remove both of them from the list
				delete(g.gatewayAPICache.RouteOptions, secondaryKey)
			}
		}
	}
	// for every duplicate we need to create a new name and then do a replace
	replacementMap := map[string]string{}
	for primaryKey, dups := range duplicates {
		newName := fmt.Sprintf("shared-%s", RandStringRunes(8))

		replacementMap[primaryKey] = newName

		//fmt.Printf("Duplicate Key: %s\n", primaryKey)
		for _, dup := range dups {
			//fmt.Printf("\t%d. Match: %s\n", i, dup)
			replacementMap[dup] = newName
		}

		// create a new RouteOption with the new name
		existingRO := g.gatewayAPICache.RouteOptions[primaryKey]
		existingRO.Name = newName
		g.gatewayAPICache.AddRouteOption(existingRO)
		delete(g.gatewayAPICache.RouteOptions, primaryKey)
	}

	for key, route := range g.gatewayAPICache.HTTPRoutes {
		var newRules []gwv1.HTTPRouteRule
		for _, rule := range route.Spec.Rules {
			for _, filter := range rule.Filters {
				if filter.ExtensionRef != nil && filter.ExtensionRef.Kind == "RouteOption" {
					nameNamespace := fmt.Sprintf("%s/%s", route.Namespace, filter.ExtensionRef.Name)
					newName, found := replacementMap[nameNamespace]
					if found {
						filter.ExtensionRef.Name = gwv1.ObjectName(newName)
					}
				}
			}
			newRules = append(newRules, rule)
		}
		route.Spec.Rules = newRules
		g.gatewayAPICache.HTTPRoutes[key] = route
	}
}
