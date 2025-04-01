package domain

import (
	"strings"

	"sigs.k8s.io/yaml"
)

type GatewayAPICache struct {
	YamlObjects         []*YAMLWrapper
	HTTPRoutes          map[string]*HTTPRouteWrapper
	RouteOptions        map[string]*RouteOptionWrapper
	VirtualHostOptions  map[string]*VirtualHostOptionWrapper
	ListenerOptions     map[string]*ListenerOptionWrapper
	HTTPListenerOptions map[string]*HTTPListenerOptionWrapper
	DirectResponses     map[string]*DirectResponseWrapper

	Upstreams    map[string]*UpstreamWrapper
	AuthConfigs  map[string]*AuthConfigWrapper
	Gateways     map[string]*GatewayWrapper
	ListenerSets map[string]*ListenerSetWrapper
}

func (g *GatewayAPICache) GetGateway(name string, namespace string) *GatewayWrapper {
	if g.Gateways == nil {
		return nil
	}
	return g.Gateways[NameNamespaceIndex(name, namespace)]
}
func (g *GatewayAPICache) AddGateway(gw *GatewayWrapper) {
	if g.Gateways == nil {
		g.Gateways = make(map[string]*GatewayWrapper)
	}
	g.Gateways[gw.NameIndex()] = gw
}
func (g *GatewayAPICache) AddDirectResponse(d *DirectResponseWrapper) {
	if g.DirectResponses == nil {
		g.DirectResponses = make(map[string]*DirectResponseWrapper)
	}
	g.DirectResponses[d.NameIndex()] = d
}
func (g *GatewayAPICache) AddYAML(y *YAMLWrapper) {
	if g.YamlObjects == nil {
		g.YamlObjects = []*YAMLWrapper{}
	}
	g.YamlObjects = append(g.YamlObjects, y)
}

func (g *GatewayAPICache) AddHTTPRoute(route *HTTPRouteWrapper) {
	if g.HTTPRoutes == nil {
		g.HTTPRoutes = make(map[string]*HTTPRouteWrapper)
	}
	g.HTTPRoutes[route.NameIndex()] = route
}
func (g *GatewayAPICache) AddRouteOption(r *RouteOptionWrapper) {
	if g.RouteOptions == nil {
		g.RouteOptions = make(map[string]*RouteOptionWrapper)
	}
	g.RouteOptions[r.NameIndex()] = r
}
func (g *GatewayAPICache) AddVirtualHostOption(v *VirtualHostOptionWrapper) {
	if g.VirtualHostOptions == nil {
		g.VirtualHostOptions = make(map[string]*VirtualHostOptionWrapper)
	}
	g.VirtualHostOptions[v.NameIndex()] = v
}
func (g *GatewayAPICache) AddListenerOption(l *ListenerOptionWrapper) {
	if g.ListenerOptions == nil {
		g.ListenerOptions = make(map[string]*ListenerOptionWrapper)
	}
	g.ListenerOptions[l.NameIndex()] = l
}
func (g *GatewayAPICache) AddHTTPListenerOption(h *HTTPListenerOptionWrapper) {
	if g.HTTPListenerOptions == nil {
		g.HTTPListenerOptions = make(map[string]*HTTPListenerOptionWrapper)
	}
	g.HTTPListenerOptions[h.NameIndex()] = h
}
func (g *GatewayAPICache) AddUpstream(u *UpstreamWrapper) {
	if g.Upstreams == nil {
		g.Upstreams = make(map[string]*UpstreamWrapper)
	}
	g.Upstreams[u.NameIndex()] = u
}
func (g *GatewayAPICache) AddAuthConfig(a *AuthConfigWrapper) {
	if g.AuthConfigs == nil {
		g.AuthConfigs = make(map[string]*AuthConfigWrapper)
	}
	g.AuthConfigs[a.NameIndex()] = a
}
func (g *GatewayAPICache) AddListenerSet(l *ListenerSetWrapper) {
	if g.ListenerSets == nil {
		g.ListenerSets = make(map[string]*ListenerSetWrapper)
	}
	g.ListenerSets[l.NameIndex()] = l
}

func (g *GatewayAPICache) HasItems() bool {
	if len(g.HTTPRoutes) > 0 {
		return true
	}
	if len(g.RouteOptions) > 0 {
		return true
	}
	if len(g.VirtualHostOptions) > 0 {
		return true
	}
	if len(g.Upstreams) > 0 {
		return true
	}
	if len(g.AuthConfigs) > 0 {
		return true
	}
	if len(g.ListenerSets) > 0 {
		return true
	}
	if len(g.HTTPListenerOptions) > 0 {
		return true
	}
	if len(g.ListenerOptions) > 0 {
		return true
	}
	if len(g.DirectResponses) > 0 {
		return true
	}
	// if there are only yaml objects then skip because we didnt change anything in the file

	return false
}

func (g *GatewayAPICache) ToString() (string, error) {
	output := ""

	for _, y := range g.YamlObjects {
		output += "\n---\n" + y.Yaml + "\n"
	}

	for _, op := range g.Upstreams {
		m, err := yaml.Marshal(op.Upstream)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}

	for _, op := range g.HTTPRoutes {
		m, err := yaml.Marshal(op.HTTPRoute)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}
	for _, op := range g.RouteOptions {
		m, err := yaml.Marshal(op.RouteOption)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}
	for _, op := range g.AuthConfigs {
		m, err := yaml.Marshal(op.AuthConfig)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}

	for _, op := range g.VirtualHostOptions {
		m, err := yaml.Marshal(op.VirtualHostOption)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}

	for _, op := range g.ListenerOptions {
		m, err := yaml.Marshal(op.ListenerOption)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}
	for _, op := range g.HTTPListenerOptions {
		m, err := yaml.Marshal(op.HttpListenerOption)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}
	for _, op := range g.ListenerSets {
		m, err := yaml.Marshal(op.XListenerSet)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}
	for _, op := range g.DirectResponses {
		m, err := yaml.Marshal(op.DirectResponse)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}

	// need to remove a few values
	//  creationTimestamp: null
	// status: {}
	// status:
	// parents: null
	output = strings.ReplaceAll(output, "  creationTimestamp: null\n", "")
	output = strings.ReplaceAll(output, "status:\n", "")
	output = strings.ReplaceAll(output, "parents: null\n", "")
	output = strings.ReplaceAll(output, "status: {}\n", "")
	output = strings.ReplaceAll(output, "\n\n\n", "\n")
	output = strings.ReplaceAll(output, "\n\n", "\n")
	output = strings.ReplaceAll(output, "spec: {}\n", "")

	// TODO remove leading and trailing ---
	// log.Printf("%s", output)
	return output, nil
}
