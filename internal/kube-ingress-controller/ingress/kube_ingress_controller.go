package ingress

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	kubeplugin "github.com/solo-io/gloo/pkg/plugins/kubernetes"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	v1beta1listers "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/rest"

	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/kubecontroller"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	defaultVirtualService = "default"

	GlooIngressClass = "gloo"

	ownerAnnotationKey = "generated_by"
)

type IngressController struct {
	errors             chan error
	useAsGlobalIngress bool

	ingressLister v1beta1listers.IngressLister
	configObjects storage.Interface
	runFunc       func(stop <-chan struct{})
	// just a random uuid used to mark
	// resources created by us as "ours"
	generatedBy string
}

func NewIngressController(cfg *rest.Config,
	configStore storage.Interface,
	resyncDuration time.Duration,
	useAsGlobalIngress bool) (*IngressController, error) {
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube clientset: %v", err)
	}

	// attempt to register configObjects if they don't exist
	if err := configStore.V1().Register(); err != nil && !storage.IsAlreadyExists(err) {
		return nil, errors.Wrap(err, "failed to register configObjects")
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, resyncDuration)
	ingressInformer := kubeInformerFactory.Extensions().V1beta1().Ingresses()

	c := &IngressController{
		errors:             make(chan error),
		useAsGlobalIngress: useAsGlobalIngress,

		ingressLister: ingressInformer.Lister(),
		configObjects: configStore,
		generatedBy:   "kube-ingress-controller",
	}

	kubeController := kubecontroller.NewController("kube-ingress-controller", kubeClient,
		kubecontroller.NewLockingSyncHandler(c.syncGlooResourcesWithIngresses),
		ingressInformer.Informer())

	c.runFunc = func(stop <-chan struct{}) {
		go kubeInformerFactory.Start(stop)
		go kubeController.Run(2, stop)
		// refresh every minute
		tick := time.Tick(time.Minute)
		go func() {
			for {
				select {
				case <-tick:
					c.syncGlooResourcesWithIngresses()
				case <-stop:
					return
				}
			}
		}()
		<-stop
		log.Printf("ingress controller stopped")
	}

	return c, nil
}

func (c *IngressController) Run(stop <-chan struct{}) {
	c.runFunc(stop)
}

func (c *IngressController) Error() <-chan error {
	return c.errors
}

func (c *IngressController) syncGlooResourcesWithIngresses() {
	if err := c.syncGlooResources(); err != nil {
		c.errors <- err
	}
}

func (c *IngressController) syncGlooResources() error {
	desiredUpstreams, desiredVirtualServices, err := c.generateDesiredResources()
	if err != nil {
		return fmt.Errorf("failed to generate desired configObjects: %v", err)
	}
	actualUpstreams, actualVirtualServices, err := c.getActualResources()
	if err != nil {
		return fmt.Errorf("failed to list actual configObjects: %v", err)
	}
	if err := c.syncUpstreams(desiredUpstreams, actualUpstreams); err != nil {
		return fmt.Errorf("failed to sync actual with desired upstreams: %v", err)
	}
	if err := c.syncVirtualServices(desiredVirtualServices, actualVirtualServices); err != nil {
		return fmt.Errorf("failed to sync actual with desired virtualServices: %v", err)
	}
	return nil
}

func (c *IngressController) getActualResources() ([]*v1.Upstream, []*v1.VirtualService, error) {
	upstreams, err := c.configObjects.V1().Upstreams().List()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get upstream crd list: %v", err)
	}
	var ourUpstreams []*v1.Upstream
	for _, us := range upstreams {
		if us.Metadata != nil && us.Metadata.Annotations[ownerAnnotationKey] == c.generatedBy {
			// our upstream, we supervise it
			ourUpstreams = append(ourUpstreams, us)
		}
	}
	virtualServices, err := c.configObjects.V1().VirtualServices().List()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get virtual service crd list: %v", err)
	}
	var ourvServices []*v1.VirtualService
	for _, vService := range virtualServices {
		if vService.Metadata != nil && vService.Metadata.Annotations[ownerAnnotationKey] == c.generatedBy {
			// our vService, we supervise it
			ourvServices = append(ourvServices, vService)
		}
	}
	return ourUpstreams, ourvServices, nil
}

func (c *IngressController) generateDesiredResources() ([]*v1.Upstream, []*v1.VirtualService, error) {
	ingressList, err := c.ingressLister.List(labels.Everything())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list ingresses: %v", err)
	}
	upstreamsByName := make(map[string]*v1.Upstream)
	routesByHostName := make(map[string][]*v1.Route)
	sslsByHostName := make(map[string]*v1.SSLConfig)
	// deterministic way to sort ingresses
	sort.SliceStable(ingressList, func(i, j int) bool {
		return strings.Compare(ingressList[i].Name, ingressList[j].Name) > 0
	})
	for _, ingress := range ingressList {
		// only care if it's our ingress class, or we're the global default
		if !isOurIngress(c.useAsGlobalIngress, ingress) {
			continue
		}
		// configure ssl for each host
		for _, tls := range ingress.Spec.TLS {
			if len(tls.Hosts) == 0 {
				sslsByHostName[defaultVirtualService] = &v1.SSLConfig{SecretRef: tls.SecretName}
			}
			for _, host := range tls.Hosts {
				sslsByHostName[host] = &v1.SSLConfig{SecretRef: tls.SecretName}
			}
		}
		// default virtualservice
		if ingress.Spec.Backend != nil {
			us := c.newUpstreamFromBackend(ingress.Namespace, *ingress.Spec.Backend)
			if _, ok := routesByHostName[defaultVirtualService]; ok {
				log.Warnf("default backend was redefined in ingress %v, ignoring", ingress.Name)
			} else {
				routesByHostName[defaultVirtualService] = []*v1.Route{
					{
						Matcher: &v1.Route_RequestMatcher{
							RequestMatcher: &v1.RequestMatcher{
								Path: &v1.RequestMatcher_PathPrefix{
									PathPrefix: "/",
								},
							},
						},
						SingleDestination: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &v1.UpstreamDestination{
									Name: us.Name,
								},
							},
						},
					},
				}
				// add upstream for default backend
				upstreamsByName[us.Name] = us
			}
		}
		for _, rule := range ingress.Spec.Rules {
			c.addRoutesAndUpstreams(ingress.Namespace, rule, upstreamsByName, routesByHostName)
		}
	}
	uniqueVirtualServices := make(map[string]*v1.VirtualService)
	for host, routes := range routesByHostName {
		// sort routes by path length
		// equal length sorted by string compare
		// longest routes should come first
		sortRoutes(routes)
		// TODO: evaluate
		// set default virtualservice to match *
		domains := []string{host}
		if host == defaultVirtualService {
			domains[0] = "*"
		}
		uniqueVirtualServices[host] = &v1.VirtualService{
			Name: host,
			// kubernetes only supports a single domain per virtualservice
			Domains:   domains,
			Routes:    routes,
			SslConfig: sslsByHostName[host],
			// mark the virtualservice as ours
			Metadata: &v1.Metadata{
				Annotations: map[string]string{
					ownerAnnotationKey: c.generatedBy,
				},
			},
		}
	}
	var (
		upstreams       []*v1.Upstream
		virtualServices []*v1.VirtualService
	)
	for _, us := range upstreamsByName {
		upstreams = append(upstreams, us)
	}
	for _, virtualService := range uniqueVirtualServices {
		virtualServices = append(virtualServices, virtualService)
	}
	return upstreams, virtualServices, nil
}

func getPathStr(matcher *v1.Route_RequestMatcher) string {
	switch path := matcher.RequestMatcher.Path.(type) {
	case *v1.RequestMatcher_PathPrefix:
		return path.PathPrefix
	case *v1.RequestMatcher_PathRegex:
		return path.PathRegex
	case *v1.RequestMatcher_PathExact:
		return path.PathExact
	}
	return ""
}

func sortRoutes(routes []*v1.Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		p1 := getPathStr(routes[i].Matcher.(*v1.Route_RequestMatcher))
		p2 := getPathStr(routes[j].Matcher.(*v1.Route_RequestMatcher))
		l1 := len(p1)
		l2 := len(p2)
		if l1 == l2 {
			return strings.Compare(p1, p2) < 0
		}
		// longer = comes first
		return l1 > l2
	})
}

func (c *IngressController) syncUpstreams(desiredUpstreams, actualUpstreams []*v1.Upstream) error {
	var (
		upstreamsToCreate []*v1.Upstream
		upstreamsToUpdate []*v1.Upstream
	)
	for _, desiredUpstream := range desiredUpstreams {
		var update bool
		for i, actualUpstream := range actualUpstreams {
			if desiredUpstream.Name == actualUpstream.Name {
				// modify existing upstream
				desiredUpstream.Metadata = actualUpstream.GetMetadata()
				update = true
				if !desiredUpstream.Equal(actualUpstream) {
					// only actually update if the spec has changed
					upstreamsToUpdate = append(upstreamsToUpdate, desiredUpstream)
				}
				// remove it from the list we match against
				actualUpstreams = append(actualUpstreams[:i], actualUpstreams[i+1:]...)
				break
			}
		}
		if !update {
			// desired was not found, mark for creation
			upstreamsToCreate = append(upstreamsToCreate, desiredUpstream)
		}
	}
	for _, us := range upstreamsToCreate {
		log.Debugf("creating upstream %v", us.Name)
		// TODO: think about caring about already exists errors
		// This workaround is necessary because the service discovery may be running and creating upstreams
		if _, err := c.configObjects.V1().Upstreams().Create(us); err != nil && !storage.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create upstream crd %s: %v", us.Name, err)
		}
	}
	for _, us := range upstreamsToUpdate {
		log.Debugf("updating upstream %v", us.Name)
		if _, err := c.configObjects.V1().Upstreams().Update(us); err != nil {
			return fmt.Errorf("failed to update upstream crd %s: %v", us.Name, err)
		}
	}
	// only remaining are no longer desired, delete em!
	for _, us := range actualUpstreams {
		log.Debugf("deleting upstream %v", us.Name)
		if err := c.configObjects.V1().Upstreams().Delete(us.Name); err != nil {
			return fmt.Errorf("failed to update upstream crd %s: %v", us.Name, err)
		}
	}
	return nil
}

func (c *IngressController) syncVirtualServices(desiredVirtualServices, actualVirtualServices []*v1.VirtualService) error {
	var (
		virtualServicesToCreate []*v1.VirtualService
		virtualServicesToUpdate []*v1.VirtualService
	)
	for _, desiredVirtualService := range desiredVirtualServices {
		var update bool
		for i, actualVirtualService := range actualVirtualServices {
			if desiredVirtualService.Name == actualVirtualService.Name {
				// modify existing virtualService
				desiredVirtualService.Metadata = actualVirtualService.GetMetadata()
				update = true
				// only actually update if the spec has changed
				if !desiredVirtualService.Equal(actualVirtualService) {
					virtualServicesToUpdate = append(virtualServicesToUpdate, desiredVirtualService)
				}
				// remove it from the list we match against
				actualVirtualServices = append(actualVirtualServices[:i], actualVirtualServices[i+1:]...)
				break
			}
		}
		if !update {
			// desired was not found, mark for creation
			virtualServicesToCreate = append(virtualServicesToCreate, desiredVirtualService)
		}
	}
	for _, virtualService := range virtualServicesToCreate {
		log.Printf("creating virtualservice %v", virtualService.Name)
		if _, err := c.configObjects.V1().VirtualServices().Create(virtualService); err != nil {
			return fmt.Errorf("failed to create virtualService crd %s: %v", virtualService.Name, err)
		}
	}
	for _, virtualService := range virtualServicesToUpdate {
		log.Printf("updating virtualservice %v", virtualService.Name)
		if _, err := c.configObjects.V1().VirtualServices().Update(virtualService); err != nil {
			return fmt.Errorf("failed to update upstream crd %s: %v", virtualService.Name, err)
		}
	}
	// only remaining are no longer desired, delete em!
	for _, virtualService := range actualVirtualServices {
		log.Printf("deleting virtualservice %v", virtualService.Name)
		if err := c.configObjects.V1().VirtualServices().Delete(virtualService.Name); err != nil {
			return fmt.Errorf("failed to update upstream crd %s: %v", virtualService.Name, err)
		}
	}
	return nil
}

func (c *IngressController) addRoutesAndUpstreams(namespace string, rule v1beta1.IngressRule, upstreams map[string]*v1.Upstream, routes map[string][]*v1.Route) {
	if rule.HTTP == nil {
		return
	}
	for _, path := range rule.HTTP.Paths {
		generatedUpstream := c.newUpstreamFromBackend(namespace, path.Backend)
		upstreams[generatedUpstream.Name] = generatedUpstream
		host := rule.Host
		if host == "" {
			host = defaultVirtualService
		}
		routes[rule.Host] = append(routes[rule.Host], &v1.Route{
			Matcher: &v1.Route_RequestMatcher{
				RequestMatcher: &v1.RequestMatcher{
					Path: &v1.RequestMatcher_PathRegex{
						PathRegex: path.Path,
					},
				},
			},
			SingleDestination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: &v1.UpstreamDestination{
						Name: generatedUpstream.Name,
					},
				},
			},
		})
	}
}

func (c *IngressController) newUpstreamFromBackend(namespace string, backend v1beta1.IngressBackend) *v1.Upstream {
	return &v1.Upstream{
		Name: UpstreamName(namespace, backend.ServiceName, backend.ServicePort),
		Type: kubeplugin.UpstreamTypeKube,
		Spec: kubeplugin.EncodeUpstreamSpec(kubeplugin.UpstreamSpec{
			ServiceName:      backend.ServiceName,
			ServiceNamespace: namespace,
			ServicePort:      backend.ServicePort.IntVal,
		}),
		// mark the upstream as ours
		Metadata: &v1.Metadata{
			Annotations: map[string]string{
				ownerAnnotationKey: c.generatedBy,
			},
		},
	}
}

func isOurIngress(useAsGlobalIngress bool, ingress *v1beta1.Ingress) bool {
	return useAsGlobalIngress || ingress.Annotations["kubernetes.io/ingress.class"] == GlooIngressClass
}

func UpstreamName(serviceNamespace, serviceName string, servicePort intstr.IntOrString) string {
	return fmt.Sprintf("%s-%s-%v", serviceNamespace, serviceName, servicePort.String())
}
