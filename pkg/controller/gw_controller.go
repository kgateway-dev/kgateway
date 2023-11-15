package controller

import (
	"context"
	"slices"

	"github.com/solo-io/gloo/projects/gateway2/deployer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	api "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	GatewayAutoDeployAnnotationKey = "gateway2.solo.io/auto-deploy"
)

type gatewayReconciler struct {
	cli           client.Client
	className     api.ObjectName
	autoProvision bool

	scheme   *runtime.Scheme
	deployer *deployer.Deployer
	kick     func(ctx context.Context)
}

func (r *gatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("gw", req.NamespacedName)
	log.V(1).Info("reconciling request", "req", req)
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
		// no need to do nothing as we have owner refs, so children will be deleted.
		log.Info("gateway deleted, no need for reconciling")
		return ctrl.Result{}, nil
	}

	log.Info("reconciling gateway")
	objs, err := r.deployer.GetObjsToDeploy(ctx, &gw)
	if err != nil {
		return ctrl.Result{}, err
	}
	// update gw status: find the name of the service we own, and see if it update the status with it
	result := ctrl.Result{}
	for _, obj := range objs {
		if svc, ok := obj.(*corev1.Service); ok {
			err := updateStatus(ctx, r.cli, &gw, &svc.ObjectMeta)
			if err != nil {
				log.Error(err, "failed to update status")
				result.Requeue = true
			}
		}
	}

	err = r.deployer.DeployObjs(ctx, objs, r.cli)
	if err != nil {
		return result, err
	}
	r.kick(ctx)

	return result, nil
}

func updateStatus(ctx context.Context, cli client.Client, gw *api.Gateway, svcmd *metav1.ObjectMeta) error {
	svcnns := client.ObjectKey{
		Namespace: svcmd.Namespace,
		Name:      svcmd.Name,
	}
	var svc corev1.Service
	if err := cli.Get(ctx, svcnns, &svc); err != nil {
		return client.IgnoreNotFound(err)
	}

	// make sure we own this service
	controller := metav1.GetControllerOf(&svc)
	if controller == nil {
		return nil
	}

	if gw.UID != controller.UID {
		return nil
	}

	// update gateway addresses in the status

	desiredAddresses := getDesiredAddresses(&svc)
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

func getDesiredAddresses(svc *corev1.Service) []api.GatewayStatusAddress {
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			return nil
		}
		var ret []api.GatewayStatusAddress

		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.Hostname != "" {
				t := api.HostnameAddressType
				ret = append(ret, api.GatewayStatusAddress{
					Type:  &t,
					Value: ing.Hostname,
				})
			}
			if ing.IP != "" {
				t := api.IPAddressType
				ret = append(ret, api.GatewayStatusAddress{
					Type:  &t,
					Value: ing.IP,
				})
			}
		}
		return ret
	}

	var ret []api.GatewayStatusAddress
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

	return ret
}
