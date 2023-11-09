package discovery

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/xds"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ Translator = new(edgeLegacyTranslator)
)

// Translator is the interface that discovery components must implement to be used by the discovery controller
// They are responsible for translating Kubernetes resources into the intermediary representation
type Translator interface {
	ReconcilePod(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
	ReconcileService(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
	ReconcileEndpoints(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
}

func NewTranslator(cli client.Client, inputChannels *xds.XdsInputChannels) Translator {
	return &edgeLegacyTranslator{
		cli:           cli,
		inputChannels: inputChannels,
		// snapshot: &OutputSnapshot{},
	}
}

// edgeLegacyTranslator is an implementation of discovery translation that relies on the Gloo Edge
// EDS and UDS implementations. These operate as a batch and are known to not be performant.
type edgeLegacyTranslator struct {
	cli           client.Client
	inputChannels *xds.XdsInputChannels
}

func (e *edgeLegacyTranslator) ReconcilePod(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return e.reconcileAll(ctx)
}

func (e *edgeLegacyTranslator) ReconcileService(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return e.reconcileAll(ctx)
}

func (e *edgeLegacyTranslator) ReconcileEndpoints(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return e.reconcileAll(ctx)
}

func (e *edgeLegacyTranslator) reconcileAll(ctx context.Context) (ctrl.Result, error) {
	// TODO:
	// 1. List resources (services, endpoints, pods) using dynamic client
	// 2. Feed resources into EDS/UDS methods from Gloo Edge, which produce Gloo Endpoints and Upstreams
	// 3. Store the snapshot
	// 4. Signal that a new snapshot is available

	svcList := corev1.ServiceList{}
	if err := e.cli.List(ctx, &svcList); err != nil {
		return ctrl.Result{}, err
	}

	epList := corev1.EndpointsList{}
	if err := e.cli.List(ctx, &epList); err != nil {
		return ctrl.Result{}, err
	}

	podList := corev1.PodList{}
	if err := e.cli.List(ctx, &podList); err != nil {
		return ctrl.Result{}, err
	}

	plugin := kubeplugin.DefaultUpstreamConverter()

	// POINTER MAPS?!?!?!?!?
	usMap := make(map[*core.ResourceRef]*kubernetes.UpstreamSpec)
	usTotalList := v1.UpstreamList{}
	for _, svc := range svcList.Items {
		svc := svc
		usList := plugin.UpstreamsForService(ctx, &svc)
		usTotalList = append(usTotalList, usList...)
		for _, us := range usList {
			kubeUpstream, ok := us.GetUpstreamType().(*v1.Upstream_Kube)
			// only care about kube upstreams
			if !ok {
				continue
			}
			usMap[us.GetMetadata().Ref()] = kubeUpstream.Kube
		}
	}

	endpoints, _, _ := kubeplugin.FilterEndpoints(
		ctx,
		"gloo-system",
		translateObjectList(epList.Items),
		translateObjectList(svcList.Items),
		translateObjectList(podList.Items),
		usMap,
	)

	e.inputChannels.UpdateDiscoveryInputs(ctx, xds.DiscoveryInputs{
		Upstreams: usTotalList,
		Endpoints: endpoints,
	})

	// Send across endpoints and upstreams
	return ctrl.Result{}, nil
}

func (e *edgeLegacyTranslator) reconcileAll2(ctx context.Context) (ctrl.Result, error) {
	// TODO:
	// 1. List resources (services, endpoints, pods) using dynamic client
	// 2. Feed resources into EDS/UDS methods from Gloo Edge, which produce Gloo Endpoints and Upstreams
	// 3. Store the snapshot
	// 4. Signal that a new snapshot is available

	svcList := corev1.ServiceList{}
	if err := e.cli.List(ctx, &svcList); err != nil {
		return ctrl.Result{}, err
	}

	epList := corev1.EndpointsList{}
	if err := e.cli.List(ctx, &epList); err != nil {
		return ctrl.Result{}, err
	}

	podList := corev1.PodList{}
	if err := e.cli.List(ctx, &podList); err != nil {
		return ctrl.Result{}, err
	}

	plugin := kubeplugin.DefaultUpstreamConverter()

	// POINTER MAPS?!?!?!?!?
	usMap := make(map[*core.ResourceRef]*kubernetes.UpstreamSpec)
	usTotalList := v1.UpstreamList{}
	for _, svc := range svcList.Items {
		var ep corev1.Endpoints
		err := e.cli.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: svc.Namespace}, &ep)
		// log err but continue with empty endpoints.
		err = client.IgnoreNotFound(err)
		if err != nil {
			//TODO:	log
		}

		svc := svc
		usList := plugin.UpstreamsForService(ctx, &svc)
		usTotalList = append(usTotalList, usList...)
		for _, us := range usList {
			kubeUpstream, ok := us.GetUpstreamType().(*v1.Upstream_Kube)
			// only care about kube upstreams
			if !ok {
				continue
			}
			usMap[us.GetMetadata().Ref()] = kubeUpstream.Kube
		}
	}

	endpoints, _, _ := kubeplugin.FilterEndpoints(
		ctx,
		"gloo-system",
		translateObjectList(epList.Items),
		translateObjectList(svcList.Items),
		translateObjectList(podList.Items),
		usMap,
	)

	e.inputChannels.UpdateDiscoveryInputs(ctx, xds.DiscoveryInputs{
		Upstreams: usTotalList,
		Endpoints: endpoints,
	})

	// Send across endpoints and upstreams
	return ctrl.Result{}, nil
}

func translateObjectList[T any](list []T) []*T {
	var out []*T
	for _, item := range list {
		item := item
		out = append(out, &item)
	}
	return out
}
