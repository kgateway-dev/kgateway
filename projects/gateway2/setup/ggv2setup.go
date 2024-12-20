package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/solo-io/gloo/pkg/utils/envutils"
	"github.com/solo-io/gloo/pkg/utils/setuputils"
	gloostatusutils "github.com/solo-io/gloo/pkg/utils/statusutils"
	gateway "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/controller"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glookubev1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/kubernetes"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/shared"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/krt"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var settingsGVR = glookubev1.SchemeGroupVersion.WithResource("settings")

func createKubeClient(restConfig *rest.Config) (istiokube.Client, error) {
	restCfg := istiokube.NewClientConfigForRestConfig(restConfig)
	client, err := istiokube.NewClient(restCfg, "")
	if err != nil {
		return nil, err
	}
	istiokube.EnableCrdWatcher(client)
	return client, nil
}

func getInitialSettings(ctx context.Context, c istiokube.Client, nns types.NamespacedName) *glookubev1.Settings {
	// get initial settings
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("getting initial settings. gvr: %v", settingsGVR)

	i, err := c.Dynamic().Resource(settingsGVR).Namespace(nns.Namespace).Get(ctx, nns.Name, metav1.GetOptions{})
	if err != nil {
		logger.Panicf("failed to get initial settings: %v", err)
		return nil
	}
	logger.Infof("got initial settings")

	var empty glookubev1.Settings
	out := &empty
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(i.UnstructuredContent(), out)
	if err != nil {
		logger.Panicf("failed converting unstructured into settings: %v", i)
		return nil
	}
	return out
}

func StartGGv2(ctx context.Context,
	setupOpts *bootstrap.SetupOpts,
	uccBuilder krtcollections.UniquelyConnectedClientsBulider,
	extraPlugins []extensionsplug.Plugin,
) error {
	restConfig := ctrl.GetConfigOrDie()

	return StartGGv2WithConfig(ctx, setupOpts, restConfig, uccBuilder, extraPlugins, setuputils.SetupNamespaceName())
}

func StartGGv2WithConfig(ctx context.Context,
	setupOpts *bootstrap.SetupOpts,
	restConfig *rest.Config,
	uccBuilder krtcollections.UniquelyConnectedClientsBulider,
	extraPlugins []extensionsplug.Plugin,
	settingsNns types.NamespacedName,
) error {
	ctx = contextutils.WithLogger(ctx, "k8s")

	logger := contextutils.LoggerFrom(ctx)
	logger.Info("starting gloo gateway")

	kubeClient, err := createKubeClient(restConfig)
	if err != nil {
		return err
	}

	initialSettings := getInitialSettings(ctx, kubeClient, settingsNns)
	if initialSettings == nil {
		return fmt.Errorf("initial settings not found")
	}

	logger.Info("creating krt collections")
	augmentedPods := krtcollections.NewPodsCollection(ctx, kubeClient, setupOpts.KrtDebugger)
	setting := krtutil.SetupCollectionDynamic[glookubev1.Settings](
		ctx,
		kubeClient,
		settingsGVR,
		krt.WithName("GlooSettings"))

	augmentedPodsForUcc := augmentedPods
	if envutils.IsEnvTruthy("DISABLE_POD_LOCALITY_XDS") {
		augmentedPodsForUcc = nil
	}

	ucc := uccBuilder(ctx, setupOpts.KrtDebugger, augmentedPodsForUcc)

	settingsSingle := krt.NewSingleton(func(ctx krt.HandlerContext) *glookubev1.Settings {
		s := krt.FetchOne(ctx, setting,
			krt.FilterObjectName(settingsNns))
		if s != nil {
			return *s
		}
		return nil
	}, krt.WithName("GlooSettingsSingleton"))

	logger.Info("creating reporter")
	kubeGwStatusReporter := NewGenericStatusReporter(kubeClient, defaults.KubeGatewayReporter)

	glooReporter := NewGenericStatusReporter(kubeClient, defaults.GlooReporter)

	logger.Info("initializing controller")

	krtOpts := krtutil.NewKrtOptions(ctx.Done(), setupOpts.KrtDebugger)

	c, err := controller.NewControllerBuilder(ctx, controller.StartConfig{
		ExtraPlugins:         extraPlugins,
		RestConfig:           restConfig,
		SetupOpts:            setupOpts,
		KubeGwStatusReporter: kubeGwStatusReporter,
		GlooStatusReporter:   glooReporter,
		Client:               kubeClient,
		AugmentedPods:        augmentedPods,
		UniqueClients:        ucc,

		InitialSettings: initialSettings,
		Settings:        settingsSingle,
		// Dev flag may be useful for development purposes; not currently tied to any user-facing API
		Dev:        os.Getenv("LOG_LEVEL") == "debug",
		KrtOptions: krtOpts,
	})
	if err != nil {
		logger.Error("failed initializing controller: ", err)
		return err
	}
	/// no collections after this point

	logger.Info("waiting for cache sync")
	kubeClient.RunAndWait(ctx.Done())
	setting.Synced().WaitUntilSynced(ctx.Done())

	logger.Info("starting controller")
	return c.Start(ctx)
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

		// check if resource is an internal upstream. if so skip it..
		if kubernetes.IsFakeKubeUpstream(resource.GetMetadata().GetName()) {
			continue
		}
		// check if resource is an internal upstream. Internal upstreams have ':' in their names so
		// the cannot be written to the cluster. if so skip it..
		if strings.IndexRune(resource.GetMetadata().GetName(), ':') >= 0 {
			continue
		}

		status := g.StatusFromReport(report, subresourceStatuses)
		status = trimStatus(status)

		resourceStatus := g.statusClient.GetStatus(resource)

		if status.Equal(resourceStatus) {
			// TODO: find a way to log this but it is noisy currently due to once per second status sync
			// see: projects/gateway2/proxy_syncer/kube_gw_translator_syncer.go#syncStatus(...)
			// and its call site in projects/gateway2/proxy_syncer/proxy_syncer.go
			// logger.Debugf("skipping report for %v as it has not changed", resource.GetMetadata().Ref())
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
	key := resources.Kind(resourceToWrite)
	crd, ok := kindToCrd[key]
	if !ok {
		err := fmt.Errorf("no crd found for kind %v", key)
		contextutils.LoggerFrom(ctx).DPanic(err)
		return err
	}
	ns := resourceToWrite.GetMetadata().GetNamespace()
	name := resourceToWrite.GetMetadata().GetName()

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
	skKind := resources.Kind(resourceType)
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
	if len(status.GetReason()) > bytesPerKey {
		status.Reason = status.GetReason()[:bytesPerKey]
	}

	if len(status.GetSubresourceStatuses()) > maxKeys {
		// sort for idempotency
		keys := make([]string, 0, len(status.GetSubresourceStatuses()))
		for key := range status.GetSubresourceStatuses() {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		trimmedSubresourceStatuses := make(map[string]*core.Status, maxKeys)
		for _, key := range keys[:maxKeys] {
			trimmedSubresourceStatuses[key] = status.GetSubresourceStatuses()[key]
		}
		status.SubresourceStatuses = trimmedSubresourceStatuses
	}

	for key, childStatus := range status.GetSubresourceStatuses() {
		// divide by two so total memory usage is bounded at: (num_keys * bytes_per_key) + (num_keys / 2 * bytes_per_key / 2) + ...
		// 100 * 1024b + 50 * 512b + 25 * 256b + 12 * 128b + 6 * 64b + 3 * 32b + 1 * 16b ~= 136 kilobytes
		//
		// 2147483647 bytes is k8s -> etcd limit in grpc connection. 2147483647 / 136 ~= 15788 resources at limit before we see an issue
		// https://github.com/solo-io/solo-projects/issues/4120
		status.GetSubresourceStatuses()[key] = trimStatusForMaxSize(childStatus, bytesPerKey/2, maxKeys/2)
	}
	return status
}
