package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mitchellh/hashstructure"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
	kubev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

func (p *plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	if p.kubeShareFactory == nil {
		p.kubeShareFactory = getInformerFactory(p.kube)
	}
	opts = opts.WithDefaults()

	return newEndpointsWatcher(p.kube, p.kubeShareFactory, upstreamsToTrack).watch(writeNamespace, opts)
}

type edsWatcher struct {
	kube             kubernetes.Interface
	upstreams        map[core.ResourceRef]*kubeplugin.UpstreamSpec
	kubeShareFactory KubePluginSharedFactory
}

func newEndpointsWatcher(kube kubernetes.Interface, kubeShareFactory KubePluginSharedFactory, upstreams v1.UpstreamList) *edsWatcher {
	upstreamSpecs := make(map[core.ResourceRef]*kubeplugin.UpstreamSpec)
	for _, us := range upstreams {
		kubeUpstream, ok := us.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Kube)
		// only care about kube upstreams
		if !ok {
			continue
		}
		upstreamSpecs[us.Metadata.Ref()] = kubeUpstream.Kube
	}
	return &edsWatcher{
		kube:             kube,
		upstreams:        upstreamSpecs,
		kubeShareFactory: kubeShareFactory,
	}
}

func (c *edsWatcher) List(writeNamespace string, opts clients.ListOpts) (v1.EndpointList, error) {
	endpoints, err := c.kubeShareFactory.EndpointsLister().List(labels.SelectorFromSet(opts.Selector))
	if err != nil {
		return nil, err
	}

	pods, err := c.kubeShareFactory.PodsLister().List(labels.SelectorFromSet(opts.Selector))
	if err != nil {
		return nil, err
	}

	return filterEndpoints(opts.Ctx, writeNamespace, endpoints, pods, c.upstreams), nil
}

func (c *edsWatcher) watch(writeNamespace string, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	watch := c.kubeShareFactory.Subscribe()

	endpointsChan := make(chan v1.EndpointList)
	errs := make(chan error)
	updateResourceList := func() {
		list, err := c.List(writeNamespace, clients.ListOpts{
			Ctx:      opts.Ctx,
			Selector: opts.Selector,
		})
		if err != nil {
			errs <- err
			return
		}
		endpointsChan <- list
	}

	go func() {
		defer c.kubeShareFactory.Unsubscribe(watch)
		defer close(endpointsChan)
		defer close(errs)

		updateResourceList()
		for {
			select {
			case _, ok := <-watch:
				if !ok {
					return
				}
				updateResourceList()
			case <-opts.Ctx.Done():
				return
			}
		}

	}()
	return endpointsChan, errs, nil
}

func filterEndpoints(ctx context.Context, writeNamespace string, kubeEndpoints []*kubev1.Endpoints, pods []*kubev1.Pod, upstreams map[core.ResourceRef]*kubeplugin.UpstreamSpec) v1.EndpointList {
	var endpoints v1.EndpointList

	logger := contextutils.LoggerFrom(contextutils.WithLogger(ctx, "kubernetes_eds"))

	type epkey struct {
		address string
		port    uint32
	}
	endpointsMap := make(map[epkey][]*core.ResourceRef)

	// for each upstream
	for usRef, spec := range upstreams {
		// find each matching endpoint
		for _, eps := range kubeEndpoints {
			if eps.Namespace != spec.ServiceNamespace || eps.Name != spec.ServiceName {
				continue
			}
			for _, subset := range eps.Subsets {
				var port uint32
				for _, p := range subset.Ports {
					if spec.TargetPort == uint32(p.Port) {
						port = uint32(p.Port)
						break
					}
				}
				if port == 0 {
					logger.Warnf("upstream %v: port %v not found for service %v", usRef.Key(), spec.TargetPort, spec.ServiceName)
					continue
				}
				for _, addr := range subset.Addresses {
					if len(spec.Selector) != 0 {
						// determine whether labels for the owner of this ip (pod) matches the spec
						podLabels, err := getPodLabelsForIp(addr.IP, pods)
						if err != nil {
							// pod not found for ip? what's that about?
							logger.Warnf("error for upstream %v service %v: %v", usRef.Key(), spec.ServiceName, err)
							continue
						}
						if !labels.AreLabelsInWhiteList(spec.Selector, podLabels) {
							continue
						}
						// pod hasn't been assigned address yet
						if addr.IP == "" {
							continue
						}
					}
					key := epkey{addr.IP, port}
					copyRef := usRef
					endpointsMap[key] = append(endpointsMap[key], &copyRef)

				}
			}
		}
	}

	for addr, refs := range endpointsMap {

		// sort refs for idempotency
		sort.Slice(refs, func(i, j int) bool { return refs[i].Key() < refs[j].Key() })

		hash, _ := hashstructure.Hash([]interface{}{refs, addr}, nil)
		dnsname := strings.Map(func(r rune) rune {
			if '0' <= r && r <= '9' {
				return r
			}
			if 'a' <= r && r <= 'z' {
				return r
			}
			return '-'
		}, addr.address)
		endpointName := fmt.Sprintf("ep-%v-%v-%x", dnsname, addr.port, hash)
		ep := createEndpoint(writeNamespace, endpointName, refs, addr.address, addr.port)
		endpoints = append(endpoints, ep)
	}

	// sort refs for idempotency
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].Metadata.Name < endpoints[j].Metadata.Name })

	return endpoints
}

func createEndpoint(namespace, name string, upstreams []*core.ResourceRef, address string, port uint32) *v1.Endpoint {
	return &v1.Endpoint{
		Metadata: core.Metadata{
			Namespace: namespace,
			Name:      name,
		},
		Upstreams: upstreams,
		Address:   address,
		Port:      port,
	}
}

func getPodLabelsForIp(ip string, pods []*kubev1.Pod) (map[string]string, error) {
	for _, pod := range pods {
		if pod.Status.PodIP == ip && pod.Status.Phase == kubev1.PodRunning {
			return pod.Labels, nil
		}
	}
	return nil, errors.Errorf("running pod not found with ip %v", ip)
}
