package kubernetes

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strings"

	errors "github.com/rotisserie/eris"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	corecache "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type podMap struct {
	// ipMap will record pods which
	// - pod.Status.Phase is kubev1.PodRunning
	// - pod.Status.PodIP != ""
	// - !pod.Spec.HostNetwork
	// and use pod.Status.PodIP as map key
	ipMap map[string]*kubev1.Pod

	// metaMap will record all pods and use
	// `pod.GetName()+"/"+pod.GetNamespace()` as map key
	metaMap map[string]*kubev1.Pod
}

func generatePodsMap(pods []*kubev1.Pod) *podMap {
	podsIPMap := make(map[string]*kubev1.Pod, len(pods))
	podsMetaMap := make(map[string]*kubev1.Pod, len(pods))
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		podsMetaMap[pod.GetName()+"/"+pod.GetNamespace()] = pod
		if pod.Status.Phase != kubev1.PodRunning {
			continue
		}
		if pod.Status.PodIP == "" {
			continue
		}
		if pod.Spec.HostNetwork {
			// we can't tell pods apart if they are all on the host network.
			continue
		}
		podsIPMap[pod.Status.PodIP] = pod
	}
	return &podMap{podsIPMap, podsMetaMap}
}

func (pm *podMap) getPodLabelsForIp(ip string, podName, podNamespace string) (map[string]string, error) {
	if podName != "" && podNamespace != "" {
		if p, ok := pm.metaMap[podName+"/"+podNamespace]; ok {
			return p.GetLabels(), nil
		}
	}
	if ip != "" {
		if p, ok := pm.ipMap[ip]; ok {
			return p.GetLabels(), nil
		}
	}

	return nil, errors.Errorf("running pod not found with IP %v", ip)
}

func (p *plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {

	kubeFactory := func(namespaces []string) KubePluginSharedFactory {
		return getInformerFactory(opts.Ctx, p.kube, namespaces)
	}
	watcher, err := newEndpointWatcherForUpstreams(kubeFactory, p.kubeCoreCache, writeNamespace, upstreamsToTrack, opts, p.settings)
	if err != nil {
		return nil, nil, err
	}
	return watcher.watch(writeNamespace, opts)
}

func newEndpointWatcherForUpstreams(kubeFactoryFactory func(ns []string) KubePluginSharedFactory, kubeCoreCache corecache.KubeCoreCache, writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts, settings *v1.Settings) (*edsWatcher, error) {
	var namespaces []string

	if settingsutil.IsAllNamespacesFromSettings(settings) {
		namespaces = []string{metav1.NamespaceAll}
	} else {
		nsSet := map[string]bool{}
		for _, upstream := range upstreamsToTrack {
			svcNs := upstream.GetKube().GetServiceNamespace()
			// only care about kube upstreams
			if svcNs == "" {
				continue
			}
			nsSet[svcNs] = true
		}
		for ns := range nsSet {
			namespaces = append(namespaces, ns)
		}
	}

	kubeFactory := kubeFactoryFactory(namespaces)
	// this can take a bit of time some make sure we are still in business
	if opts.Ctx.Err() != nil {
		return nil, opts.Ctx.Err()
	}
	opts = opts.WithDefaults()

	return newEndpointsWatcher(kubeCoreCache, namespaces, kubeFactory, upstreamsToTrack), nil
}

type edsWatcher struct {
	upstreams         map[*core.ResourceRef]*kubeplugin.UpstreamSpec
	kubeShareFactory  KubePluginSharedFactory
	kubeCoreCache     corecache.KubeCoreCache
	namespaces        []string
	lastEndpointsHash uint64
}

func newEndpointsWatcher(kubeCoreCache corecache.KubeCoreCache, namespaces []string, kubeShareFactory KubePluginSharedFactory, upstreams v1.UpstreamList) *edsWatcher {
	upstreamSpecs := make(map[*core.ResourceRef]*kubeplugin.UpstreamSpec)
	for _, us := range upstreams {
		kubeUpstream, ok := us.GetUpstreamType().(*v1.Upstream_Kube)
		// only care about kube upstreams
		if !ok {
			continue
		}
		upstreamSpecs[us.GetMetadata().Ref()] = kubeUpstream.Kube
	}
	return &edsWatcher{
		upstreams:        upstreamSpecs,
		kubeShareFactory: kubeShareFactory,
		kubeCoreCache:    kubeCoreCache,
		namespaces:       namespaces,
	}
}

func (c *edsWatcher) List(writeNamespace string, opts clients.ListOpts) (v1.EndpointList, error) {
	var endpointList []*kubev1.Endpoints
	var serviceList []*kubev1.Service
	var podList []*kubev1.Pod
	ctx := contextutils.WithLogger(opts.Ctx, "kubernetes_eds")
	var warnsToLog []string

	for _, ns := range c.namespaces {
		if c.kubeCoreCache.NamespacedServiceLister(ns) == nil {
			// this namespace is not watched, ignore it.
			warnsToLog = append(warnsToLog, fmt.Sprintf("namespace %v is not watched, and has upstreams pointing to it", ns))
			continue
		}
		services, err := c.kubeCoreCache.NamespacedServiceLister(ns).List(labels.SelectorFromSet(opts.Selector))
		if err != nil {
			return nil, err
		}
		serviceList = append(serviceList, services...)
		pods, err := c.kubeCoreCache.NamespacedPodLister(ns).List(labels.SelectorFromSet(opts.Selector))
		if err != nil {
			return nil, err
		}
		podList = append(podList, pods...)

		endpoints, err := c.kubeShareFactory.EndpointsLister(ns).List(labels.SelectorFromSet(opts.Selector))
		if err != nil {
			return nil, err
		}
		endpointList = append(endpointList, endpoints...)
	}

	eps, warns, errsToLog := filterEndpoints(ctx, writeNamespace, endpointList, serviceList, podList, c.upstreams)

	warnsToLog = append(warnsToLog, warns...)

	hasher := fnv.New64()
	for _, ep := range eps {
		result, err := ep.Hash(hasher)
		if err != nil {
			return nil, err
		}
		err = binary.Write(hasher, binary.LittleEndian, result)
		if err != nil {
			return nil, err
		}
	}
	hash := hasher.Sum64()

	if c.lastEndpointsHash == hash {
		return eps, nil
	}
	c.lastEndpointsHash = hash

	// We return and log any messages here later because endpoints are constantly being updated / triggering the
	// EDS loop by the endpoint shared informer. By ensuring the hash is different, we know the endpoints are
	// functionally different and logging any new information might mean different / new information.
	//
	// This change helps avoid disk filling up from endless logs and cluttering the logs with the same message.
	logger := contextutils.LoggerFrom(ctx)
	for _, warnToLog := range warnsToLog {
		logger.Warn(warnToLog)
	}
	for _, errToLog := range errsToLog {
		logger.Error(errToLog)
	}

	return eps, nil
}

// Returns true for when configured for Istio integration where endpoints must
// be defined by IP address rather than hostnames. For details, see:
// * https://github.com/solo-io/gloo/issues/6195
func isIstioIntegrationEnabled() bool {
	lookupResult, found := os.LookupEnv("ENABLE_ISTIO_INTEGRATION")
	return found && strings.ToLower(lookupResult) == "true"
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
		select {
		case <-opts.Ctx.Done():
			return
		case endpointsChan <- list:
		}
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

type Epkey struct {
	Address     string
	Port        uint32
	Name        string
	Namespace   string
	UpstreamRef *core.ResourceRef
	// While we can use the upstream ref to get the upstream and service, if there are too many upstreams that could be slow.
	IsHeadless bool
}

// Returns first matching port in the namespace and boolean value of true if the
// service has a single port.
// If no matching port is found, returns nil and false.
func findPortForService(services []*kubev1.Service, spec *kubeplugin.UpstreamSpec) (*kubev1.ServicePort, bool) {
	for _, svc := range services {
		if svc.Namespace != spec.GetServiceNamespace() || svc.Name != spec.GetServiceName() {
			continue
		}
		for _, port := range svc.Spec.Ports {
			if spec.GetServicePort() == uint32(port.Port) {
				return &port, len(svc.Spec.Ports) == 1
			}
		}
	}
	return nil, false
}

func getServiceFromUpstreamSpec(spec *kubeplugin.UpstreamSpec, services []*kubev1.Service) *kubev1.Service {
	for _, svc := range services {
		if svc.Namespace == spec.GetServiceNamespace() && svc.Name == spec.GetServiceName() {
			return svc
		}
	}
	return nil
}

func filterEndpoints(
	_ context.Context, // do not use for logging! return logging messages as strings and log them after hashing (see https://github.com/solo-io/gloo/issues/3761)
	writeNamespace string,
	kubeEndpoints []*kubev1.Endpoints,
	services []*kubev1.Service,
	pods []*kubev1.Pod,
	upstreams map[*core.ResourceRef]*kubeplugin.UpstreamSpec,
) (v1.EndpointList, []string, []string) {
	var endpoints v1.EndpointList

	var warnsToLog, errorsToLog []string
	endpointsMap := make(map[Epkey][]*core.ResourceRef)
	podMap := generatePodsMap(pods)

	istioIntegrationEnabled := isIstioIntegrationEnabled()

	// for each upstream
	for usRef, spec := range upstreams {
		kubeServicePort, singlePortService := findPortForService(services, spec)
		if kubeServicePort == nil {
			errorsToLog = append(errorsToLog, fmt.Sprintf("upstream %v: port %v not found for service %v", usRef.Key(), spec.GetServicePort(), spec.GetServiceName()))
			continue
		}

		svc := getServiceFromUpstreamSpec(spec, services)
		// TODO: Investigate possible deprecation of ClusterIPs in newer k8s versions https://github.com/solo-io/gloo/issues/7830
		isHeadlessSvc := svc.Spec.ClusterIP == "None"
		// Istio uses the service's port for routing requests
		// Headless services don't have a cluster IP, so we'll resort to pod IP endpoints
		if istioIntegrationEnabled && !isHeadlessSvc {
			hostname := fmt.Sprintf("%v.%v", spec.GetServiceName(), spec.GetServiceNamespace())
			copyRef := *usRef
			key := Epkey{
				Address:     hostname,
				Port:        uint32(kubeServicePort.Port),
				Name:        spec.GetServiceName(),
				Namespace:   spec.GetServiceNamespace(),
				UpstreamRef: &copyRef,
				IsHeadless:  isHeadlessSvc,
			}
			endpointsMap[key] = append(endpointsMap[key], &copyRef)
			continue
		}

		// find each matching endpoint
		for _, eps := range kubeEndpoints {
			if eps.Namespace != spec.GetServiceNamespace() || eps.Name != spec.GetServiceName() {
				continue
			}
			for _, subset := range eps.Subsets {
				port := findFirstPortInEndpointSubsets(subset, singlePortService, kubeServicePort)
				if port == 0 {
					warnsToLog = append(warnsToLog, fmt.Sprintf("upstream %v: port %v not found for service %v in endpoint %v", usRef.Key(), spec.GetServicePort(), spec.GetServiceName(), subset))
					continue
				}

				warnings := processSubsetAddresses(subset, spec, podMap, usRef, port, endpointsMap, isHeadlessSvc)
				warnsToLog = append(warnsToLog, warnings...)
			}
		}
	}

	endpoints = generateFilteredEndpointList(endpointsMap, services, podMap, writeNamespace, endpoints, istioIntegrationEnabled)

	return endpoints, warnsToLog, errorsToLog
}

func processSubsetAddresses(subset kubev1.EndpointSubset, spec *kubeplugin.UpstreamSpec, pods *podMap, usRef *core.ResourceRef, port uint32, endpointsMap map[Epkey][]*core.ResourceRef, isHeadlessService bool) []string {
	var warnings []string
	for _, addr := range subset.Addresses {
		var podName, podNamespace string
		targetRef := addr.TargetRef
		if targetRef != nil {
			if targetRef.Kind == "Pod" {
				podName = targetRef.Name
				podNamespace = targetRef.Namespace
			}
		}
		if len(spec.GetSelector()) != 0 {
			// determine whether labels for the owner of this ip (pod) matches the spec
			podLabels, err := pods.getPodLabelsForIp(addr.IP, podName, podNamespace)
			if err != nil {
				// pod not found for IP? what's that about?
				warnings = append(warnings, fmt.Sprintf("error for upstream %v service %v: %v", usRef.Key(), spec.GetServiceName(), err))
				continue
			}
			if !labels.SelectorFromSet(spec.GetSelector()).Matches(labels.Set(podLabels)) {
				continue
			}
			// pod hasn't been assigned address yet
			if addr.IP == "" {
				continue
			}
		}
		key := Epkey{addr.IP, port, podName, podNamespace, usRef, isHeadlessService}
		copyRef := *usRef
		endpointsMap[key] = append(endpointsMap[key], &copyRef)
	}
	return warnings
}

func findFirstPortInEndpointSubsets(subset kubev1.EndpointSubset, singlePortService bool, kubeServicePort *kubev1.ServicePort) uint32 {
	var port uint32
	for _, p := range subset.Ports {
		// if the endpoint port is not named, it implies that
		// the kube service only has a single unnamed port as well.
		switch {
		case singlePortService:
			port = uint32(p.Port)
		case p.Name == kubeServicePort.Name:
			port = uint32(p.Port)
			break
		}
	}
	return port
}

func generateFilteredEndpointList(
	endpointsMap map[Epkey][]*core.ResourceRef,
	services []*kubev1.Service,
	pods *podMap,
	writeNamespace string,
	endpoints v1.EndpointList,
	istioIntegrationEnabled bool) v1.EndpointList {
	for addr, refs := range endpointsMap {

		// sort refs for idempotency
		sort.Slice(refs, func(i, j int) bool { return refs[i].Key() < refs[j].Key() })
		hasher := fnv.New64()
		hasher.Write([]byte(fmt.Sprintf("%+v", addr)))
		dnsname := strings.Map(func(r rune) rune {
			if '0' <= r && r <= '9' {
				return r
			}
			if 'a' <= r && r <= 'z' {
				return r
			}
			return '-'
		}, addr.Address)
		endpointName := fmt.Sprintf("ep-%v-%v-%x", dnsname, addr.Port, hasher.Sum64())

		var ep *v1.Endpoint
		// While istio integration requires the Service VIP, headless services require the pod IP, as there is no Cluster IP
		if istioIntegrationEnabled && !addr.IsHeadless {
			// Istio integration requires assigning endpoints the Kub service VIP rather than pod address
			service, _ := getServiceForHostname(addr.Address, addr.Name, addr.Namespace, services)
			ep = createEndpoint(writeNamespace, endpointName, refs, service.Spec.ClusterIP, addr.Port, service.GetObjectMeta().GetLabels()) // TODO: labels may be nil
		} else {
			podLabels, _ := pods.getPodLabelsForIp(addr.Address, addr.Name, addr.Namespace)
			ep = createEndpoint(writeNamespace, endpointName, refs, addr.Address, addr.Port, podLabels)
		}
		endpoints = append(endpoints, ep)
	}

	// sort refs for idempotency
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].GetMetadata().GetName() < endpoints[j].GetMetadata().GetName()
	})
	return endpoints
}

func createEndpoint(namespace, name string, upstreams []*core.ResourceRef, address string, port uint32, labels map[string]string) *v1.Endpoint {
	return &v1.Endpoint{
		Metadata: &core.Metadata{
			Namespace: namespace,
			Name:      name,
			Labels:    labels,
		},
		Upstreams: upstreams,
		Address:   address,
		Port:      port,
		// TODO: add locality info
	}
}

func getServiceForHostname(hostname string, serviceName, serviceNamespace string, services []*kubev1.Service) (*kubev1.Service, error) {

	for _, service := range services {
		if serviceName != "" && serviceNamespace != "" {
			// no need for heuristics!
			if serviceName == service.Name && serviceNamespace == service.Namespace {
				return service, nil
			}
		}

		serviceHostname := fmt.Sprintf("%v.%v", service.Name, service.Namespace)

		if serviceHostname != hostname {
			continue
		}

		return service, nil
	}

	return nil, errors.Errorf("service not found with hostname %v", hostname)
}
