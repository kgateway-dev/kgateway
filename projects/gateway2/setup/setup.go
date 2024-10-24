package setup

import (
	"context"
	"fmt"
	"sort"

	"errors"

	"github.com/solo-io/gloo/pkg/utils/envutils"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"
	gateway "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	"github.com/solo-io/gloo/projects/gloo/constants"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/shared"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func createKubeClient() (istiokube.Client, error) {
	restCfg := istiokube.NewClientConfigForRestConfig(ctrl.GetConfigOrDie())
	client, err := istiokube.NewClient(restCfg, "")
	if err != nil {
		return nil, err
	}
	istiokube.EnableCrdWatcher(client)
	return client, nil
}

func StartGGv2(ctx context.Context,
	setupOpts *bootstrap.SetupOpts,
	extensionsFactory extensions.K8sGatewayExtensionsFactory,
	pluginRegistryFactory func(opts registry.PluginOpts) plugins.PluginRegistryFactory) error {
	ctx = contextutils.WithLogger(ctx, "k8s")

	kubeClient, err := createKubeClient()
	if err != nil {
		return err
	}

	go kubeClient.RunAndWait(ctx.Done())
	// create agumented pods
	pods := krtcollections.NewPodsCollection(ctx, kubeClient)

	setting := proxy_syncer.SetupCollectionDynamic[glookubev1.Settings](
		ctx,
		kubeClient,
		glookubev1.SchemeGroupVersion.WithResource("settings"),
		krt.WithName("GlooSettings"))

	setupNamespaceName := setuputils.SetupNamespaceName()

	settingsSingle := krt.NewSingleton[glookubev1.Settings](
		func(ctx krt.HandlerContext) *glookubev1.Settings {
			s := krt.FetchOne(ctx, setting,
				krt.FilterObjectName(setupNamespaceName))
			if s != nil {
				return *s
			}
			return nil
		})

	settingsSingle.AsCollection().Synced().WaitUntilSynced(ctx.Done())

	serviceClient := kclient.New[*corev1.Service](kubeClient)
	services := krt.WrapClient(serviceClient, krt.WithName("Services"))

	pluginOpts := registry.PluginOpts{
		Ctx:                     ctx,
		SidecarOnGatewayEnabled: envutils.IsEnvTruthy(constants.IstioInjectionEnabled),
		SvcCollection:           services,
	}

	kubeGwStatusReporter := NewGenericStatusReporter(kubeClient, defaults.KubeGatewayReporter)
	glooReporter := NewGenericStatusReporter(kubeClient, defaults.GlooReporter)
	return controller.Start(ctx, controller.StartConfig{
		ExtensionsFactory:    extensionsFactory,
		SetupOpts:            setupOpts,
		KubeGwStatusReporter: kubeGwStatusReporter,
		Translator:           setup.TranslatorFactory{PluginRegistry: pluginRegistryFactory(pluginOpts)},
		GlooStatusReporter:   glooReporter,
		Client:               kubeClient,
		Pods:                 pods,
		Settings:             settingsSingle,
		// Useful for development purposes; not currently tied to any user-facing API
		Dev: false,
	})
}

type genericStatusReporter struct {
	client               istiokube.Client
	kubeGwStatusReporter reporter.StatusReporter
	statusClient         resources.StatusClient
}

func NewGenericStatusReporter(client istiokube.Client, r string) reporter.StatusReporter {
	statusReporterNamespace := gloostatusutils.GetStatusReporterNamespaceOrDefault("gloo-system")
	statusClient := gloostatusutils.GetStatusClientForNamespace(statusReporterNamespace)

	kubeGwStatusReporter := reporter.NewReporter(
		r,
		statusClient,
	)
	return &genericStatusReporter{client: client, kubeGwStatusReporter: kubeGwStatusReporter, statusClient: statusClient}
}

// StatusFromReport implements reporter.StatusReporter.
func (g *genericStatusReporter) StatusFromReport(report reporter.Report, subresourceStatuses map[string]*core.Status) *core.Status {
	return g.kubeGwStatusReporter.StatusFromReport(report, subresourceStatuses)
}

// WriteReports implements reporter.StatusReporter.
func (g *genericStatusReporter) WriteReports(ctx context.Context, resourceErrs reporter.ResourceReports, subresourceStatuses map[string]*core.Status) error {
	ctx = contextutils.WithLogger(ctx, "reporter")
	logger := contextutils.LoggerFrom(ctx)

	var merr error

	// copy the map so we can iterate over the copy, deleting resources from
	// the original map if they are not found/no longer exist.
	resourceErrsCopy := make(reporter.ResourceReports, len(resourceErrs))
	for resource, report := range resourceErrs {
		resourceErrsCopy[resource] = report
	}

	for resource, report := range resourceErrsCopy {

		status := g.StatusFromReport(report, subresourceStatuses)
		status = trimStatus(status)

		resourceStatus := g.statusClient.GetStatus(resource)

		if status.Equal(resourceStatus) {
			logger.Debugf("skipping report for %v as it has not changed", resource.GetMetadata().Ref())
			continue
		}

		resourceToWrite := resources.Clone(resource).(resources.InputResource)
		g.statusClient.SetStatus(resourceToWrite, status)
		writeErr := g.attemptUpdateStatus(ctx, resourceToWrite, status)

		if k8serrors.IsNotFound(writeErr) {
			logger.Debugf("did not write report for %v : %v because resource was not found", resourceToWrite.GetMetadata().Ref(), status)
			delete(resourceErrs, resource)
			continue
		}

		if writeErr != nil {
			err := fmt.Errorf("failed to write status %v for resource %v: %w", status, resource.GetMetadata().GetName(), writeErr)
			logger.Warn(err)
			merr = errors.Join(merr, err)
			continue
		}
		logger.Debugf("wrote report for %v : %v", resource.GetMetadata().Ref(), status)

	}
	return merr
}

func (g *genericStatusReporter) attemptUpdateStatus(ctx context.Context, resourceToWrite resources.InputResource, statusToWrite *core.Status) error {
	crd, ok := kindToCrd[resources.Kind(resourceToWrite)]
	if !ok {
		err := fmt.Errorf("no crd found for kind %v", resources.Kind(resourceToWrite))
		contextutils.LoggerFrom(ctx).DPanic(err)
		return err
	}
	ns := resourceToWrite.GetMetadata().Namespace
	name := resourceToWrite.GetMetadata().Name

	data, err := shared.GetJsonPatchData(ctx, resourceToWrite)
	if err != nil {
		return fmt.Errorf("error getting status json patch data: %w", err)
	}

	_, err = g.client.Dynamic().Resource(crd.GroupVersion().WithResource(crd.CrdMeta.Plural)).Namespace(ns).Patch(ctx, name, types.JSONPatchType, data, metav1.PatchOptions{})
	return err
}

var _ reporter.StatusReporter = &genericStatusReporter{}

var kindToCrd = map[string]crd.Crd{}

func add(crd crd.Crd, resourceType resources.InputResource) {
	skKind := resources.Kind(new(gateway.RouteOption))
	kindToCrd[skKind] = crd
}

func init() {
	add(gateway.RouteOptionCrd, new(gateway.RouteOption))
	add(gateway.VirtualHostOptionCrd, new(gateway.VirtualHostOption))
	add(gloov1.ProxyCrd, new(gloov1.Proxy))
	add(gloov1.UpstreamCrd, new(gloov1.Upstream))
	// add(rlv1alpha1.RateLimitCrd, new(rlv1alpha1.RateLimit))
	// add(rlv1alpha1.RateLimitCrd, new(rlv1alpha1.RateLimit))
}

func trimStatus(status *core.Status) *core.Status {
	// truncate status reason to a kilobyte, with max 100 keys in subresource statuses
	return trimStatusForMaxSize(status, reporter.MaxStatusBytes, reporter.MaxStatusKeys)
}

func trimStatusForMaxSize(status *core.Status, bytesPerKey, maxKeys int) *core.Status {
	if status == nil {
		return nil
	}
	if len(status.Reason) > bytesPerKey {
		status.Reason = status.Reason[:bytesPerKey]
	}

	if len(status.SubresourceStatuses) > maxKeys {
		// sort for idempotency
		keys := make([]string, 0, len(status.SubresourceStatuses))
		for key := range status.SubresourceStatuses {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		trimmedSubresourceStatuses := make(map[string]*core.Status, maxKeys)
		for _, key := range keys[:maxKeys] {
			trimmedSubresourceStatuses[key] = status.SubresourceStatuses[key]
		}
		status.SubresourceStatuses = trimmedSubresourceStatuses
	}

	for key, childStatus := range status.SubresourceStatuses {
		// divide by two so total memory usage is bounded at: (num_keys * bytes_per_key) + (num_keys / 2 * bytes_per_key / 2) + ...
		// 100 * 1024b + 50 * 512b + 25 * 256b + 12 * 128b + 6 * 64b + 3 * 32b + 1 * 16b ~= 136 kilobytes
		//
		// 2147483647 bytes is k8s -> etcd limit in grpc connection. 2147483647 / 136 ~= 15788 resources at limit before we see an issue
		// https://github.com/solo-io/solo-projects/issues/4120
		status.SubresourceStatuses[key] = trimStatusForMaxSize(childStatus, bytesPerKey/2, maxKeys/2)
	}
	return status
}
