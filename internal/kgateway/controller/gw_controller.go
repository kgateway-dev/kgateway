package controller

import (
	"context"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	api "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

const (
	GatewayAutoDeployAnnotationKey = "gateway.kgateway.dev/auto-deploy"
)

type gatewayReconciler struct {
	cli           client.Client
	autoProvision bool

	controllerName string

	scheme   *runtime.Scheme
	deployer *deployer.Deployer
	metrics  controllerMetricsRecorder
}

func (r *gatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rErr error) {
	log := log.FromContext(ctx).WithValues("gw", req.NamespacedName)
	log.V(1).Info("reconciling request", "req", req)

	if r.metrics != nil {
		defer r.metrics.reconcileStart()(rErr)
	}

	// check if we need to auto deploy the gateway
	ns := req.Namespace
	// get the namespace
	var namespace corev1.Namespace
	if err := r.cli.Get(ctx, client.ObjectKey{Name: ns}, &namespace); err != nil {
		log.Error(err, "unable to get namespace")
		return ctrl.Result{}, err
	}

	// check for the annotation:
	if !r.autoProvision && namespace.Annotations[GatewayAutoDeployAnnotationKey] != "true" {
		log.Info("namespace is not enabled for auto deploy.")
		return ctrl.Result{}, nil
	}

	var gw api.Gateway
	if err := r.cli.Get(ctx, req.NamespacedName, &gw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if gw.GetDeletionTimestamp() != nil {
		// no need to do anything as we have owner refs, so children will be deleted
		log.Info("gateway deleted, no need for reconciling")
		return ctrl.Result{}, nil
	}

	// make sure we're the right controller for this
	var gwc api.GatewayClass
	if err := r.cli.Get(
		ctx,
		client.ObjectKey{Name: string(gw.Spec.GatewayClassName)},
		&gwc,
	); err != nil {
		log.Error(err, "failed to check controller for GatewayClass")
		return ctrl.Result{}, err
	}
	if gwc.Spec.ControllerName != api.GatewayController(r.controllerName) {
		// ignore, not our GatewayClass
		return ctrl.Result{}, nil
	}

	log.Info("reconciling gateway")
	objs, err := r.deployer.GetObjsToDeploy(ctx, &gw)
	if err != nil {
		return ctrl.Result{}, err
	}
	objs = r.deployer.SetNamespaceAndOwner(&gw, objs)

	// find the name/ns of the service we own so we can grab addresses
	// from it for status
	var generatedSvc *metav1.ObjectMeta
	for _, obj := range objs {
		if svc, ok := obj.(*corev1.Service); ok {
			generatedSvc = &svc.ObjectMeta
			break
		}
	}

	// update status (whether we generated a service or not, for unmanaged)
	result := ctrl.Result{}
	err = updateStatus(ctx, r.cli, &gw, generatedSvc)
	if err != nil {
		log.Error(err, "failed to update status")
		result.Requeue = true
	}

	err = r.deployer.DeployObjs(ctx, objs)
	if err != nil {
		return result, err
	}

	return result, nil
}

func updateStatus(ctx context.Context, cli client.Client, gw *api.Gateway, svcmd *metav1.ObjectMeta) error {
	var svc *corev1.Service
	if svcmd != nil {
		svcnns := client.ObjectKey{
			Namespace: svcmd.Namespace,
			Name:      svcmd.Name,
		}

		svc = &corev1.Service{}
		if err := cli.Get(ctx, svcnns, svc); err != nil {
			return client.IgnoreNotFound(err)
		}

		// make sure we own this service
		controller := metav1.GetControllerOf(svc)
		if controller == nil {
			return nil
		}

		if gw.UID != controller.UID {
			return nil
		}
	}

	// update gateway addresses in the status
	desiredAddresses := getDesiredAddresses(gw, svc)
	actualAddresses := gw.Status.Addresses
	if slices.Equal(desiredAddresses, actualAddresses) {
		return nil
	}

	gw.Status.Addresses = desiredAddresses
	if err := cli.Status().Patch(ctx, gw, client.Merge); err != nil {
		return err
	}
	return nil
}

func getDesiredAddresses(gw *api.Gateway, svc *corev1.Service) []api.GatewayStatusAddress {
	var ret []api.GatewayStatusAddress
	seen := sets.New[api.GatewayStatusAddress]()

	if svc != nil && svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			return nil
		}
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if addr, ok := convertIngressAddr(ing); ok {
				seen.Insert(addr)
				ret = append(ret, addr)
			}
		}

		return ret
	} else if svc != nil {
		t := api.IPAddressType
		if len(svc.Spec.ClusterIPs) != 0 {
			for _, ip := range svc.Spec.ClusterIPs {
				ret = append(ret, api.GatewayStatusAddress{
					Type:  &t,
					Value: ip,
				})
			}
		} else if svc.Spec.ClusterIP != "" {
			ret = append(ret, api.GatewayStatusAddress{
				Type:  &t,
				Value: svc.Spec.ClusterIP,
			})
		}
	}

	for _, specAddr := range gw.Spec.Addresses {
		addr := api.GatewayStatusAddress{
			Type:  specAddr.Type,
			Value: specAddr.Value,
		}
		if !seen.Has(addr) {
			ret = append(ret, addr)
		}
	}

	return ret
}

func convertIngressAddr(ing corev1.LoadBalancerIngress) (api.GatewayStatusAddress, bool) {
	if ing.Hostname != "" {
		t := api.HostnameAddressType
		return api.GatewayStatusAddress{
			Type:  &t,
			Value: ing.Hostname,
		}, true
	}
	if ing.IP != "" {
		t := api.IPAddressType
		return api.GatewayStatusAddress{
			Type:  &t,
			Value: ing.IP,
		}, true
	}
	return api.GatewayStatusAddress{}, false
}
