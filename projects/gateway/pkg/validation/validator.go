package validation

import (
	"context"
	"sort"
	"sync"
	"time"

	utils2 "github.com/solo-io/gloo/pkg/utils"
	gloo_translator "github.com/solo-io/gloo/projects/gloo/pkg/translator"

	"github.com/hashicorp/go-multierror"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	skprotoutils "github.com/solo-io/solo-kit/pkg/utils/protoutils"

	"github.com/avast/retry-go"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"go.uber.org/multierr"

	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
)

type ProxyReports map[*gloov1.Proxy]*validation.ProxyReport

var (
	NotReadyErr = errors.Errorf("validation is not yet available. Waiting for first snapshot")

	RouteTableDeleteErr = func(parentVirtualServices, parentRouteTables []*core.ResourceRef) error {
		return errors.Errorf("Deletion blocked because active Routes delegate to this Route Table. Remove delegate actions to this route table from the virtual services: %v and the route tables: %v, then try again", parentVirtualServices, parentRouteTables)
	}
	VirtualServiceDeleteErr = func(parentGateways []*core.ResourceRef) error {
		return errors.Errorf("Deletion blocked because active Gateways reference this Virtual Service. Remove refs to this virtual service from the gateways: %v, then try again", parentGateways)
	}
	unmarshalErrMsg     = "could not unmarshal raw object"
	WrappedUnmarshalErr = func(err error) error {
		return errors.Wrapf(err, unmarshalErrMsg)
	}

	mValidConfig = utils2.MakeGauge("validation.gateway.solo.io/valid_config", "A boolean indicating whether gloo config is valid")
)

const (
	InvalidSnapshotErrMessage = "validation is disabled due to an invalid resource which has been written to storage. " +
		"Please correct any Rejected resources to re-enable validation."
)

var _ Validator = &validator{}

type Validator interface {
	v1.ApiSyncer
	ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (ProxyReports, *multierror.Error)
	ValidateGateway(ctx context.Context, gw *v1.Gateway, dryRun bool) (ProxyReports, error)
	ValidateVirtualService(ctx context.Context, vs *v1.VirtualService, dryRun bool) (ProxyReports, error)
	ValidateDeleteVirtualService(ctx context.Context, vs *core.ResourceRef, dryRun bool) error
	ValidateRouteTable(ctx context.Context, rt *v1.RouteTable, dryRun bool) (ProxyReports, error)
	ValidateDeleteRouteTable(ctx context.Context, rt *core.ResourceRef, dryRun bool) error
}

type validator struct {
	lock                         sync.RWMutex
	latestSnapshot               *v1.ApiSnapshot
	latestSnapshotErr            error
	translator                   translator.Translator
	validationClient             validation.ProxyValidationServiceClient
	ignoreProxyValidationFailure bool
	allowWarnings                bool
	writeNamespace               string
}

type ValidatorConfig struct {
	translator                   translator.Translator
	validationClient             validation.ProxyValidationServiceClient
	writeNamespace               string
	ignoreProxyValidationFailure bool
	allowWarnings                bool
}

func NewValidatorConfig(translator translator.Translator, validationClient validation.ProxyValidationServiceClient, writeNamespace string, ignoreProxyValidationFailure, allowWarnings bool) ValidatorConfig {
	return ValidatorConfig{
		translator:                   translator,
		validationClient:             validationClient,
		writeNamespace:               writeNamespace,
		ignoreProxyValidationFailure: ignoreProxyValidationFailure,
		allowWarnings:                allowWarnings,
	}
}

func NewValidator(cfg ValidatorConfig) *validator {
	return &validator{
		translator:                   cfg.translator,
		validationClient:             cfg.validationClient,
		writeNamespace:               cfg.writeNamespace,
		ignoreProxyValidationFailure: cfg.ignoreProxyValidationFailure,
		allowWarnings:                cfg.allowWarnings,
	}
}

func (v *validator) ready() bool {
	return v.latestSnapshot != nil
}

func (v *validator) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	snapCopy := snap.Clone()
	gatewaysByProxy := utils.GatewaysByProxyName(snap.Gateways)
	var errs error
	for proxyName, gatewayList := range gatewaysByProxy {
		_, reports := v.translator.Translate(ctx, proxyName, v.writeNamespace, snap, gatewayList)
		if err := reports.Validate(); err != nil {
			errs = multierr.Append(errs, err)
		}
	}

	v.lock.Lock()
	defer v.lock.Unlock()

	v.latestSnapshotErr = errs
	v.latestSnapshot = &snapCopy

	if errs != nil {
		utils2.MeasureZero(ctx, mValidConfig)
		return errors.Wrapf(errs, InvalidSnapshotErrMessage)
	}

	utils2.MeasureOne(ctx, mValidConfig)
	return nil
}

type applyResource func(snap *v1.ApiSnapshot) (proxyNames []string, resource resources.Resource, ref *core.ResourceRef)

// update internal snapshot to handle race where a lot of resources may be deleted at once, before syncer updates
// should be called within a lock
func (v *validator) deleteFromLocalSnapshot(resource resources.Resource) {
	ref := resource.GetMetadata().Ref()
	switch resource.(type) {
	case *v1.VirtualService:
		for i, rt := range v.latestSnapshot.VirtualServices {
			if rt.Metadata.Ref().Equal(ref) {
				v.latestSnapshot.VirtualServices = append(v.latestSnapshot.VirtualServices[:i], v.latestSnapshot.VirtualServices[i+1:]...)
				break
			}
		}
	case *v1.RouteTable:
		for i, rt := range v.latestSnapshot.RouteTables {
			if rt.Metadata.Ref().Equal(ref) {
				v.latestSnapshot.RouteTables = append(v.latestSnapshot.RouteTables[:i], v.latestSnapshot.RouteTables[i+1:]...)
				break
			}
		}
	}
}

func (v *validator) validateSnapshotThreadSafe(ctx context.Context, apply applyResource, dryRun bool) (ProxyReports, error) {
	// thread-safe implementation of validateSnapshot
	v.lock.Lock()
	defer v.lock.Unlock()

	return v.validateSnapshot(ctx, apply, dryRun)
}

func (v *validator) validateSnapshot(ctx context.Context, apply applyResource, dryRun bool) (ProxyReports, error) {
	// validate that a snapshot can be modified
	// should be called within a lock
	//
	// validation occurs by the following steps:
	//	1. Clone the most recent snapshot
	//	2. Apply the changes to that snapshot clone
	//	3. Validate the generated proxy of that snapshot clone by making a gRPC call to Gloo.
	//	4. If the proxy is valid, we know that the requested mutation is valid. If this request happens
	//		during a dry run, we don't want to actually apply the change, since this will modify the internal
	//		state of the validator, which is shared across requests. Therefore, only if we are not in a dry run,
	//		we apply the mutation.
	if !v.ready() {
		return nil, NotReadyErr
	}

	ctx = contextutils.WithLogger(ctx, "gateway-validator")

	snapshotClone := v.latestSnapshot.Clone()

	if v.latestSnapshotErr != nil {
		utils2.MeasureZero(ctx, mValidConfig)
		contextutils.LoggerFrom(ctx).Errorw(InvalidSnapshotErrMessage, zap.Error(v.latestSnapshotErr))
		// allow writes if storage is already broken
		return nil, nil
	}

	utils2.MeasureOne(ctx, mValidConfig)

	// verify the mutation against a snapshot clone first, only apply the change to the actual snapshot if this passes
	proxyNames, resource, ref := apply(&snapshotClone)

	gatewaysByProxy := utils.GatewaysByProxyName(snapshotClone.Gateways)

	var (
		errs         error
		proxyReports ProxyReports = map[*gloov1.Proxy]*validation.ProxyReport{}
	)
	for _, proxyName := range proxyNames {
		gatewayList := gatewaysByProxy[proxyName]
		proxy, reports := v.translator.Translate(ctx, proxyName, v.writeNamespace, &snapshotClone, gatewayList)
		validate := reports.ValidateStrict
		if v.allowWarnings {
			validate = reports.Validate
		}
		if err := validate(); err != nil {
			errs = multierr.Append(errs, errors.Wrapf(err, "could not render proxy"))
			continue
		}

		if v.validationClient == nil {
			contextutils.LoggerFrom(ctx).Warnf("skipping proxy validation check as the " +
				"Proxy validation client has not been initialized. check to ensure that the gateway and gloo processes " +
				"are configured to communicate.")
			continue
		}

		// a nil proxy may have been returned if 0 listeners were created
		if proxy == nil {
			continue
		}

		// validate the proxy with gloo
		var proxyReport *validation.ProxyValidationServiceResponse
		err := retry.Do(func() error {
			rpt, err := v.validationClient.ValidateProxy(ctx, &validation.ProxyValidationServiceRequest{Proxy: proxy})
			proxyReport = rpt
			return err
		},
			retry.Attempts(4),
			retry.Delay(250*time.Millisecond),
		)
		if err != nil {
			err = errors.Wrapf(err, "failed to communicate with Gloo Proxy validation server")
			if v.ignoreProxyValidationFailure {
				contextutils.LoggerFrom(ctx).Error(err)
			} else {
				errs = multierr.Append(errs, err)
			}
			continue
		}

		proxyReports[proxy] = proxyReport.ProxyReport
		if err := validationutils.GetProxyError(proxyReport.ProxyReport); err != nil {
			errs = multierr.Append(errs, errors.Wrapf(err, "failed to validate Proxy with Gloo validation server"))
			continue
		}
		if warnings := validationutils.GetProxyWarning(proxyReport.ProxyReport); !v.allowWarnings && len(warnings) > 0 {
			for _, warning := range warnings {
				errs = multierr.Append(errs, errors.New(warning))
			}
			continue
		}
	}

	if errs != nil {
		contextutils.LoggerFrom(ctx).Debugf("Rejected %T %v: %v", resource, ref, errs)
		return proxyReports, errors.Wrapf(errs, "validating %T %v", resource, ref)
	}

	contextutils.LoggerFrom(ctx).Debugf("Accepted %T %v", resource, ref)

	if !dryRun {
		// update internal snapshot to handle race where a lot of resources may be applied at once, before syncer updates
		apply(v.latestSnapshot)
	}

	return proxyReports, nil
}

func (v *validator) ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (ProxyReports, *multierror.Error) {
	var (
		proxyReports = ProxyReports{}
		errs         = &multierror.Error{}
	)

	v.lock.Lock()
	defer v.lock.Unlock()
	originalSnapshot := v.latestSnapshot.Clone()

	for _, item := range ul.Items {

		var itemProxyReports, err = v.processItem(ctx, item)

		errs = multierror.Append(errs, err)
		for proxy, report := range itemProxyReports {
			// ok to return final proxy reports as the latest result includes latest proxy calculated
			// for each resource, as we process incrementally, storing new state in memory as we go
			proxyReports[proxy] = report
		}
	}

	if dryRun {
		// to validate the entire list of changes against one another, each item was applied to the latestSnapshot
		// if this is a dry run, latestSnapshot needs to be reset back to it's original value without any of the changes
		v.latestSnapshot = &originalSnapshot
	}

	return proxyReports, errs
}

func (v *validator) processItem(ctx context.Context, item unstructured.Unstructured) (ProxyReports, error) {
	// process a single change in a list of changes
	//
	// when calling the specific internal validate method, dryRun and acquireLock are always false:
	// 	dryRun=false: this enables items to be validated against other items in the list
	// 	acquireLock=false: the entire list of changes are called within a single lock
	gv, err := schema.ParseGroupVersion(item.GetAPIVersion())
	if err != nil {
		return ProxyReports{}, err
	}

	itemGvk := schema.GroupVersionKind{
		Version: gv.Version,
		Group:   gv.Group,
		Kind:    item.GetKind(),
	}

	jsonBytes, err := item.MarshalJSON()
	if err != nil {
		return ProxyReports{}, err
	}

	switch itemGvk {
	case v1.GatewayGVK:
		var (
			gw v1.Gateway
		)
		if unmarshalErr := skprotoutils.UnmarshalResource(jsonBytes, &gw); unmarshalErr != nil {
			return ProxyReports{}, WrappedUnmarshalErr(unmarshalErr)
		}
		return v.validateGatewayInternal(ctx, &gw, false, false)
	case v1.VirtualServiceGVK:
		var (
			vs v1.VirtualService
		)
		if unmarshalErr := skprotoutils.UnmarshalResource(jsonBytes, &vs); unmarshalErr != nil {
			return ProxyReports{}, WrappedUnmarshalErr(unmarshalErr)
		}
		return v.validateVirtualServiceInternal(ctx, &vs, false, false)
	case v1.RouteTableGVK:
		var (
			rt v1.RouteTable
		)
		if unmarshalErr := skprotoutils.UnmarshalResource(jsonBytes, &rt); unmarshalErr != nil {
			return ProxyReports{}, WrappedUnmarshalErr(unmarshalErr)
		}
		return v.validateRouteTableInternal(ctx, &rt, false, false)
	}
	// should not happen
	return ProxyReports{}, errors.Errorf("Unknown group/version/kind, %v", itemGvk)
}

func (v *validator) ValidateVirtualService(ctx context.Context, vs *v1.VirtualService, dryRun bool) (ProxyReports, error) {
	return v.validateVirtualServiceInternal(ctx, vs, dryRun, true)
}

func (v *validator) validateVirtualServiceInternal(ctx context.Context, vs *v1.VirtualService, dryRun, acquireLock bool) (ProxyReports, error) {
	apply := func(snap *v1.ApiSnapshot) ([]string, resources.Resource, *core.ResourceRef) {
		vsRef := vs.GetMetadata().Ref()

		// TODO: move this to a function when generics become a thing
		var isUpdate bool
		for i, existingVs := range snap.VirtualServices {
			if existingVs.GetMetadata().Ref().Equal(vsRef) {
				// replace the existing virtual service in the snapshot
				snap.VirtualServices[i] = vs
				isUpdate = true
				break
			}
		}
		if !isUpdate {
			snap.VirtualServices = append(snap.VirtualServices, vs)
			snap.VirtualServices.Sort()
		}

		return proxiesForVirtualService(snap.Gateways, vs), vs, vsRef
	}

	if acquireLock {
		return v.validateSnapshotThreadSafe(ctx, apply, dryRun)
	} else {
		return v.validateSnapshot(ctx, apply, dryRun)
	}
}

func (v *validator) ValidateDeleteVirtualService(ctx context.Context, vsRef *core.ResourceRef, dryRun bool) error {
	if !v.ready() {
		return errors.Errorf("Gateway validation is yet not available. Waiting for first snapshot")
	}
	v.lock.Lock()
	defer v.lock.Unlock()
	snap := v.latestSnapshot.Clone()

	vs, err := snap.VirtualServices.Find(vsRef.Strings())
	if err != nil {
		// if it's not present in the snapshot, allow deletion
		return nil
	}

	var parentGateways []*core.ResourceRef
	snap.Gateways.Each(func(element *v1.Gateway) {
		http, ok := element.GatewayType.(*v1.Gateway_HttpGateway)
		if !ok {
			return
		}
		for _, ref := range http.HttpGateway.GetVirtualServices() {
			if ref.Equal(vsRef) {
				// this gateway points at this virtual service
				parentGateways = append(parentGateways, element.Metadata.Ref())

				break
			}
		}
	})

	if len(parentGateways) > 0 {
		err := VirtualServiceDeleteErr(parentGateways)
		if !v.allowWarnings {
			contextutils.LoggerFrom(ctx).Infof("Rejected deletion of Virtual Service %v: %v", vsRef, err)
			return err
		}
		contextutils.LoggerFrom(ctx).Warn("Allowed deletion of Virtual Service %v with warning: %v", vsRef, err)
	} else {
		contextutils.LoggerFrom(ctx).Debugw("Accepted deletion of Virtual Service %v", vsRef)
	}

	if !dryRun {
		v.deleteFromLocalSnapshot(vs)
	}
	return nil
}

func (v *validator) ValidateRouteTable(ctx context.Context, rt *v1.RouteTable, dryRun bool) (ProxyReports, error) {
	return v.validateRouteTableInternal(ctx, rt, dryRun, true)
}

func (v *validator) validateRouteTableInternal(ctx context.Context, rt *v1.RouteTable, dryRun, acquireLock bool) (ProxyReports, error) {
	apply := func(snap *v1.ApiSnapshot) ([]string, resources.Resource, *core.ResourceRef) {
		rtRef := rt.GetMetadata().Ref()

		// TODO: move this to a function when generics become a thing
		var isUpdate bool
		for i, existingRt := range snap.RouteTables {
			if existingRt.GetMetadata().Ref().Equal(rtRef) {
				// replace the existing route table in the snapshot
				snap.RouteTables[i] = rt
				isUpdate = true
				break
			}
		}
		if !isUpdate {
			snap.RouteTables = append(snap.RouteTables, rt)
			snap.RouteTables.Sort()
		}

		proxiesToConsider := proxiesForRouteTable(snap.Gateways, snap.VirtualServices, snap.RouteTables, rt)

		return proxiesToConsider, rt, rtRef
	}

	if acquireLock {
		return v.validateSnapshotThreadSafe(ctx, apply, dryRun)
	} else {
		return v.validateSnapshot(ctx, apply, dryRun)
	}
}

func (v *validator) ValidateDeleteRouteTable(ctx context.Context, rtRef *core.ResourceRef, dryRun bool) error {
	if !v.ready() {
		return errors.Errorf("Gateway validation is yet not available. Waiting for first snapshot")
	}
	v.lock.Lock()
	defer v.lock.Unlock()
	snap := v.latestSnapshot.Clone()

	rt, err := snap.RouteTables.Find(rtRef.Strings())
	if err != nil {
		// if it's not present in the snapshot, allow deletion
		return nil
	}

	refsToDelete := refSet{gloo_translator.UpstreamToClusterName(rtRef): rtRef}

	var parentVirtualServices []*core.ResourceRef
	snap.VirtualServices.Each(func(element *v1.VirtualService) {
		if routesContainRefs(element.GetVirtualHost().GetRoutes(), refsToDelete) {
			parentVirtualServices = append(parentVirtualServices, element.Metadata.Ref())
		}
	})

	var parentRouteTables []*core.ResourceRef
	snap.RouteTables.Each(func(element *v1.RouteTable) {
		if routesContainRefs(element.GetRoutes(), refsToDelete) {
			parentRouteTables = append(parentRouteTables, element.Metadata.Ref())
		}
	})

	if len(parentVirtualServices) > 0 || len(parentRouteTables) > 0 {
		err := RouteTableDeleteErr(parentVirtualServices, parentRouteTables)
		if !v.allowWarnings {
			contextutils.LoggerFrom(ctx).Debugw("Rejected deletion of Route Table %v: %v", rtRef, err)
			return err
		}
		contextutils.LoggerFrom(ctx).Warn("Allowed deletion of Route Table %v with warning: %v", rtRef, err)
	} else {
		contextutils.LoggerFrom(ctx).Debugw("Accepted Route Table deletion %v", rtRef)
	}

	if !dryRun {
		v.deleteFromLocalSnapshot(rt)
	}
	return nil
}

func (v *validator) ValidateGateway(ctx context.Context, gw *v1.Gateway, dryRun bool) (ProxyReports, error) {
	return v.validateGatewayInternal(ctx, gw, dryRun, true)
}

func (v *validator) validateGatewayInternal(ctx context.Context, gw *v1.Gateway, dryRun, acquireLock bool) (ProxyReports, error) {
	apply := func(snap *v1.ApiSnapshot) ([]string, resources.Resource, *core.ResourceRef) {
		gwRef := gw.GetMetadata().Ref()

		// TODO: move this to a function when generics become a thing
		var isUpdate bool
		for i, existingGw := range snap.Gateways {
			if existingGw.GetMetadata().Ref().Equal(gwRef) {
				// replace the existing gateway in the snapshot
				snap.Gateways[i] = gw
				isUpdate = true
				break
			}
		}
		if !isUpdate {
			snap.Gateways = append(snap.Gateways, gw)
			snap.Gateways.Sort()
		}

		proxiesToConsider := utils.GetProxyNamesForGateway(gw)

		return proxiesToConsider, gw, gwRef
	}

	if acquireLock {
		return v.validateSnapshotThreadSafe(ctx, apply, dryRun)
	} else {
		return v.validateSnapshot(ctx, apply, dryRun)
	}
}

func proxiesForVirtualService(gwList v1.GatewayList, vs *v1.VirtualService) []string {

	gatewaysByProxy := utils.GatewaysByProxyName(gwList)

	var proxiesToConsider []string

	for proxyName, gatewayList := range gatewaysByProxy {
		if gatewayListContainsVirtualService(gatewayList, vs) {
			// we only care about validating this proxy if it contains this virtual service
			proxiesToConsider = append(proxiesToConsider, proxyName)
		}
	}

	sort.Strings(proxiesToConsider)

	return proxiesToConsider
}

func proxiesForRouteTable(gwList v1.GatewayList, vsList v1.VirtualServiceList, rtList v1.RouteTableList, rt *v1.RouteTable) []string {
	affectedVirtualServices := virtualServicesForRouteTable(rt, vsList, rtList)

	affectedProxies := make(map[string]struct{})
	for _, vs := range affectedVirtualServices {
		proxiesToConsider := proxiesForVirtualService(gwList, vs)
		for _, proxy := range proxiesToConsider {
			affectedProxies[proxy] = struct{}{}
		}
	}

	var proxiesToConsider []string
	for proxy := range affectedProxies {
		proxiesToConsider = append(proxiesToConsider, proxy)
	}
	sort.Strings(proxiesToConsider)

	return proxiesToConsider
}

type refSet map[string]*core.ResourceRef

func virtualServicesForRouteTable(rt *v1.RouteTable, allVirtualServices v1.VirtualServiceList, allRoutes v1.RouteTableList) v1.VirtualServiceList {
	// this route table + its parents
	refsContainingRouteTable := refSet{gloo_translator.UpstreamToClusterName(rt.Metadata.Ref()): rt.GetMetadata().Ref()}

	// keep going until the ref list stops expanding
	for countedRefs := 0; countedRefs != len(refsContainingRouteTable); {
		countedRefs = len(refsContainingRouteTable)
		for _, route := range allRoutes {
			if routesContainRefs(route.GetRoutes(), refsContainingRouteTable) {
				refsContainingRouteTable[gloo_translator.UpstreamToClusterName(route.GetMetadata().Ref())] = route.GetMetadata().Ref()
			}
		}
	}

	var parentVirtualServices v1.VirtualServiceList
	allVirtualServices.Each(func(element *v1.VirtualService) {
		if routesContainRefs(element.GetVirtualHost().GetRoutes(), refsContainingRouteTable) {
			parentVirtualServices = append(parentVirtualServices, element)
		}
	})

	return parentVirtualServices
}

func routesContainRefs(list []*v1.Route, refs refSet) bool {
	for _, r := range list {

		delegate := r.GetDelegateAction()
		if delegate == nil {
			continue
		}

		var routeTableRef *core.ResourceRef
		// handle deprecated route table resource reference format
		// TODO(marco): remove when we remove the deprecated fields from the API
		if delegate.Namespace != "" || delegate.Name != "" {
			routeTableRef = &core.ResourceRef{
				Namespace: delegate.Namespace,
				Name:      delegate.Name,
			}
		} else {
			switch selectorType := delegate.GetDelegationType().(type) {
			case *v1.DelegateAction_Selector:
				// Selectors do not represent hard referential constraints, i.e. we can safely remove
				// a route table even when it is matches by one or more selectors. Hence, skip this check.
				continue
			case *v1.DelegateAction_Ref:
				routeTableRef = selectorType.Ref
			}
		}

		if routeTableRef == nil {
			continue
		}

		if _, ok := refs[gloo_translator.UpstreamToClusterName(routeTableRef)]; ok {
			return true
		}
	}
	return false
}

func gatewayListContainsVirtualService(gwList v1.GatewayList, vs *v1.VirtualService) bool {
	for _, gw := range gwList {
		contains, err := translator.GatewayContainsVirtualService(gw, vs)
		if err != nil {
			return false
		}
		if contains {
			return true
		}
	}

	return false
}
