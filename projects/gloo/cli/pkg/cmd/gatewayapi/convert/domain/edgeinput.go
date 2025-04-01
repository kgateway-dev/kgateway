package domain

import (
	"fmt"
	"strings"

	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"sigs.k8s.io/yaml"
)

type GlooEdgeCache struct {
	YamlObjects         []*YAMLWrapper
	RouteTables         map[string]*RouteTableWrapper
	RouteOptions        map[string]*RouteOptionWrapper
	ListenerOptions     map[string]*ListenerOptionWrapper
	HTTPListenerOptions map[string]*HTTPListenerOptionWrapper
	VirtualHostOptions  map[string]*VirtualHostOptionWrapper
	Upstreams           map[string]*UpstreamWrapper
	VirtualServices     map[string]*VirtualServiceWrapper
	Gateways            map[string]*GlooGatewayWrapper
	AuthConfigs         map[string]*AuthConfigWrapper
}

func (g *GlooEdgeCache) GetVirtualServices(name, namespace string) []*VirtualServiceWrapper {
	var response []*VirtualServiceWrapper
	if name == "" && namespace == "" {
		return response
	}
	if name == "" {
		// namespace only search
		for vsName, virtualService := range g.VirtualServices {
			split := strings.Split(vsName, "/")
			if split[0] == namespace {
				response = append(response, virtualService)
			}
		}
	} else if namespace == "" {
		for vsName, virtualService := range g.VirtualServices {
			split := strings.Split(vsName, "/")
			if split[1] == name {
				response = append(response, virtualService)
			}
		}
	} else {
		vs := g.VirtualServices[NameNamespaceIndex(namespace, name)]
		if vs != nil {
			response = append(response, vs)
		}
	}
	return response
}
func (g *GlooEdgeCache) GlooGatewayContainsVirtualService(gateway *GlooGatewayWrapper) ([]*VirtualServiceWrapper, error) {
	var response []*VirtualServiceWrapper

	for _, virtualService := range g.VirtualServices {

		var matches bool
		var err error
		if gateway.Gateway.Spec.GetHttpGateway() != nil && len(gateway.Gateway.Spec.GetHttpGateway().GetVirtualServiceNamespaces()) > 0 {
			// the HttpGatewayContainsVirtualService function requires the VS to have a namespace and metadata on the inner object which we don't have. So we do the namespace lookup ourselves
			for _, ns := range gateway.Gateway.Spec.GetHttpGateway().GetVirtualServiceNamespaces() {
				if ns == "*" {
					matches = true
					break
				}
				if ns == virtualService.Namespace {
					matches = true
					break
				}
			}
		} else {
			// this logic reqires the VS to have a namespace and metadata on the inner object which we dont have. So we do the namespace lookup ourselves
			matches, err = translator.HttpGatewayContainsVirtualService(gateway.Gateway.Spec.GetHttpGateway(), &virtualService.Spec, gateway.Gateway.Spec.Ssl)
			if err != nil {
				return nil, err
			}
		}

		if matches {
			response = append(response, virtualService)
		}
	}
	return response, nil
}

func (g *GlooEdgeCache) AddYamlObject(w *YAMLWrapper) {
	if g.YamlObjects == nil {
		g.YamlObjects = []*YAMLWrapper{}
	}
	g.YamlObjects = append(g.YamlObjects, w)
}
func (g *GlooEdgeCache) AddRouteTable(r *RouteTableWrapper) {
	if g.RouteTables == nil {
		g.RouteTables = map[string]*RouteTableWrapper{}
	}
	g.RouteTables[r.NameIndex()] = r
}
func (g *GlooEdgeCache) AddRouteOption(r *RouteOptionWrapper) {
	if g.RouteOptions == nil {
		g.RouteOptions = map[string]*RouteOptionWrapper{}
	}
	g.RouteOptions[r.NameIndex()] = r
}
func (g *GlooEdgeCache) AddListenerOption(l *ListenerOptionWrapper) {
	if g.ListenerOptions == nil {
		g.ListenerOptions = map[string]*ListenerOptionWrapper{}
	}
	g.ListenerOptions[l.NameIndex()] = l
}
func (g *GlooEdgeCache) AddHTTPListenerOption(h *HTTPListenerOptionWrapper) {
	if g.HTTPListenerOptions == nil {
		g.HTTPListenerOptions = map[string]*HTTPListenerOptionWrapper{}
	}
	g.HTTPListenerOptions[h.NameIndex()] = h
}
func (g *GlooEdgeCache) AddVirtualHostOption(v *VirtualHostOptionWrapper) {
	if g.VirtualHostOptions == nil {
		g.VirtualHostOptions = map[string]*VirtualHostOptionWrapper{}
	}
	g.VirtualHostOptions[v.NameIndex()] = v
}
func (g *GlooEdgeCache) AddUpstream(u *UpstreamWrapper) {
	if g.Upstreams == nil {
		g.Upstreams = map[string]*UpstreamWrapper{}
	}
	g.Upstreams[u.NameIndex()] = u
}

func (g *GlooEdgeCache) AddVirtualService(v *VirtualServiceWrapper) {
	if g.VirtualServices == nil {
		g.VirtualServices = map[string]*VirtualServiceWrapper{}
	}
	g.VirtualServices[v.NameIndex()] = v
}
func (g *GlooEdgeCache) AddGlooGateway(w *GlooGatewayWrapper) {
	if g.Gateways == nil {
		g.Gateways = map[string]*GlooGatewayWrapper{}
	}
	g.Gateways[w.NameIndex()] = w
}
func (g *GlooEdgeCache) AddAuthConfig(a *AuthConfigWrapper) {
	if g.AuthConfigs == nil {
		g.AuthConfigs = map[string]*AuthConfigWrapper{}
	}
	g.AuthConfigs[a.NameIndex()] = a
}

func NameNamespaceIndex(name string, namespace string) string {
	return fmt.Sprintf("%s-%s", namespace, name)
}

func (g *GlooEdgeCache) GetUpstream(name string, namespace string) *UpstreamWrapper {
	return g.Upstreams[NameNamespaceIndex(name, namespace)]
}

func (g *GlooEdgeCache) ToString() (string, error) {
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

	for _, op := range g.RouteTables {
		m, err := yaml.Marshal(op.RouteTable)
		if err != nil {
			return "", err
		}

		output += "\n---\n" + string(m)
	}
	for _, op := range g.VirtualServices {
		m, err := yaml.Marshal(op.VirtualService)
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
