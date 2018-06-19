package resolver

import (
	"k8s.io/client-go/kubernetes"

	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	staticplugin "github.com/solo-io/gloo/pkg/coreplugins/static"
	"github.com/solo-io/gloo/pkg/plugins/consul"
	kubeplugin "github.com/solo-io/gloo/pkg/plugins/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Resolver interface {
	Resolve(us *v1.Upstream) (string, error)
}

func NewResolver(kube kubernetes.Interface, client *api.Client) Resolver {
	return &resolver{Kube: kube, Consul: client}
}

type resolver struct {
	Kube   kubernetes.Interface
	Consul *api.Client
}

func (r *resolver) Resolve(us *v1.Upstream) (string, error) {
	switch us.Type {
	case kubeplugin.UpstreamTypeKube:
		return resolveKubeUpstream(r.Kube, us)
	case staticplugin.UpstreamTypeService:
		return resolveServiceUpstream(us)
	case consul.UpstreamTypeConsul:
		return resolveConsulUpstream(r.Consul, us)
	}
	// ignore other upstream types
	return "", nil
}

func resolveKubeUpstream(kube kubernetes.Interface, us *v1.Upstream) (string, error) {
	if kube == nil {
		return "", errors.New("function discovery not enabled for kubernetes upstreams. function discovery must be run in-cluster for this feature")
	}
	spec, err := kubeplugin.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return "", errors.Wrap(err, "parsing kubernetes upstream spec")
	}
	// port specified, we're good
	if spec.ServicePort != 0 {
		return fmt.Sprintf("%v.%v.svc.cluster.local:%v", spec.ServiceName, spec.ServiceNamespace, spec.ServicePort), nil
	}
	// look up port. must be single-port service
	svc, err := kube.CoreV1().Services(spec.ServiceNamespace).Get(spec.ServiceName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, "failed to get k8s service for upstream")
	}
	if len(svc.Spec.Ports) != 1 {
		return "", errors.New("service_port must be specified for k8s services with more than one port")
	}
	port := svc.Spec.Ports[0].Port
	return fmt.Sprintf("%v.%v.svc.cluster.local:%v", spec.ServiceName, spec.ServiceNamespace, port), nil
}

func resolveServiceUpstream(us *v1.Upstream) (string, error) {
	spec, err := staticplugin.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return "", errors.Wrap(err, "parsing service upstream spec")
	}
	if len(spec.Hosts) < 1 {
		return "", errors.New("at least one host required to resolve service upstream")
	}
	return fmt.Sprintf("%v:%v", spec.Hosts[0].Addr, spec.Hosts[0].Port), nil
}

func resolveConsulUpstream(client *api.Client, us *v1.Upstream) (string, error) {
	if client == nil {
		return "", errors.New("function discovery not enabled for consul upstreams. function discovery must be " +
			"configured to communicate with consul for this feature")
	}
	spec, err := consul.DecodeUpstreamSpec(us.Spec)
	if err != nil {
		return "", errors.Wrap(err, "parsing service upstream spec")
	}
	instances, _, err := client.Catalog().Service(spec.ServiceName, "", &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return "", errors.Wrap(err, "getting service from catalog")
	}

	for _, inst := range instances {
		if matchTags(spec.ServiceTags, inst.ServiceTags) {
			return fmt.Sprintf("%v:%v", inst.ServiceAddress, inst.ServicePort), nil
		}
	}

	return "", errors.Errorf("service with name %s and tags %v not found", spec.ServiceTags, spec.ServiceTags)
}

func matchTags(t1, t2 []string) bool {
	if len(t1) != len(t2) {
		return false
	}
	for _, tag1 := range t1 {
		var found bool
		for _, tag2 := range t2 {
			if tag1 == tag2 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
