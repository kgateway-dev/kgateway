package validation

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	skprotoutils "github.com/solo-io/solo-kit/pkg/utils/protoutils"

	"github.com/avast/retry-go"
	"github.com/solo-io/go-utils/protoutils"
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

	RouteTableDeleteErr = func(parentVirtualServices, parentRouteTables []core.ResourceRef) error {
		return errors.Errorf("Deletion blocked because active Routes delegate to this Route Table. Remove delegate actions to this route table from the virtual services: %v and the route tables: %v, then try again", parentVirtualServices, parentRouteTables)
	}
	VirtualServiceDeleteErr = func(parentGateways []core.ResourceRef) error {
		return errors.Errorf("Deletion blocked because active Gateways reference this Virtual Service. Remove refs to this virtual service from the gateways: %v, then try again", parentGateways)
	}
	unmarshalErrMsg     = "could not unmarshal raw object"
	WrappedUnmarshalErr = func(err error) error {
		return errors.Wrapf(err, unmarshalErrMsg)
	}
)

const (
	InvalidSnapshotErrMessage = "validation is disabled due to an invalid resource which has been written to storage. " +
		"Please correct any Rejected resources to re-enable validation."
)

type Validator interface {
	v1.ApiSyncer
	ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (ProxyReports, error)
	ValidateGateway(ctx context.Context, gw *v1.Gateway, dryRun bool) (ProxyReports, error)
	ValidateVirtualService(ctx context.Context, vs *v1.VirtualService, dryRun bool) (ProxyReports, error)
	ValidateDeleteVirtualService(ctx context.Context, vs core.ResourceRef, dryRun bool) error
	ValidateRouteTable(ctx context.Context, rt *v1.RouteTable, dryRun bool) (ProxyReports, error)
	ValidateDeleteRouteTable(ctx context.Context, rt core.ResourceRef, dryRun bool) error
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
		return errors.Wrapf(errs, InvalidSnapshotErrMessage)
	}

	return nil
}

type applyResource func(snap *v1.ApiSnapshot) (proxyNames []string, resource resources.Resource, ref core.ResourceRef)

// update internal snapshot to handle race where a lot of resources may be deleted at once, before syncer updates
// should be called within a lock
func (v *validator) deleteFromLocalSnapshot(resource resources.Resource) {
	ref := resource.GetMetadata().Ref()
	switch resource.(type) {
	case *v1.VirtualService:
		for i, rt := range v.latestSnapshot.VirtualServices {
			if rt.Metadata.Ref() == ref {
				v.latestSnapshot.VirtualServices = append(v.latestSnapshot.VirtualServices[:i], v.latestSnapshot.VirtualServices[i+1:]...)
				break
			}
		}
	case *v1.RouteTable:
		for i, rt := range v.latestSnapshot.RouteTables {
			if rt.Metadata.Ref() == ref {
				v.latestSnapshot.RouteTables = append(v.latestSnapshot.RouteTables[:i], v.latestSnapshot.RouteTables[i+1:]...)
				break
			}
		}
	}
}

func (v *validator) validateSnapshot(ctx context.Context, apply applyResource, dryRun, acquireLock bool) (ProxyReports, error) {
	if !v.ready() {
		return nil, NotReadyErr
	}

	ctx = contextutils.WithLogger(ctx, "gateway-validator")

	snap := v.latestSnapshot.Clone()

	if v.latestSnapshotErr != nil {
		contextutils.LoggerFrom(ctx).Errorw(InvalidSnapshotErrMessage, zap.Error(v.latestSnapshotErr))
		// allow writes if storage is already broken
		return nil, nil
	}

	proxyNames, resource, ref := apply(&snap)

	gatewaysByProxy := utils.GatewaysByProxyName(snap.Gateways)

	var (
		errs         error
		proxyReports ProxyReports = map[*gloov1.Proxy]*validation.ProxyReport{}
	)
	for _, proxyName := range proxyNames {
		gatewayList := gatewaysByProxy[proxyName]
		proxy, reports := v.translator.Translate(ctx, proxyName, v.writeNamespace, &snap, gatewayList)
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

			if reportData, marshalErr := protoutils.MarshalBytes(proxyReport); marshalErr == nil {
				err = errors.Wrapf(err, "%s", reportData)
			}
			errs = multierr.Append(errs, errors.Wrapf(err, "failed to validate Proxy with Gloo validation server"))
			continue
		}
	}

	if errs != nil {
		contextutils.LoggerFrom(ctx).Debugw("Rejected %T %v: %v", resource, ref, errs)
		return proxyReports, errors.Wrapf(errs, "validating %T %v", resource, ref)
	}

	contextutils.LoggerFrom(ctx).Debugw("Accepted %T %v", resource, ref)

	if !dryRun {
		// update internal snapshot to handle race where a lot of resources may be applied at once, before syncer updates
		if acquireLock {
			v.lock.Lock()
		}
		apply(v.latestSnapshot)
		if acquireLock {
			v.lock.Unlock()
		}
	}

	return proxyReports, nil
}

func (v *validator) ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (ProxyReports, error) {
	var (
		proxyReports = ProxyReports{}
		errs         = &multierror.Error{}
	)

	v.lock.Lock()
	defer v.lock.Unlock()
	snap := v.latestSnapshot.Clone()

	for _, item := range ul.Items {

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

		var itemProxyReports ProxyReports
		switch itemGvk {
		case v1.GatewayGVK:
			var (
				gw v1.Gateway
			)
			if err := skprotoutils.UnmarshalResource(jsonBytes, &gw); err != nil {
				return nil, WrappedUnmarshalErr(err)
			}
			itemProxyReports, err = v.validateGatewayInternal(ctx, &gw, false, false)
		case v1.VirtualServiceGVK:
			var (
				vs v1.VirtualService
			)
			if err := skprotoutils.UnmarshalResource(jsonBytes, &vs); err != nil {
				return nil, WrappedUnmarshalErr(err)
			}
			itemProxyReports, err = v.validateVirtualServiceInternal(ctx, &vs, false, false)
		case v1.RouteTableGVK:
			var (
				rt v1.RouteTable
			)
			if err := skprotoutils.UnmarshalResource(jsonBytes, &rt); err != nil {
				return nil, WrappedUnmarshalErr(err)
			}
			itemProxyReports, err = v.validateRouteTableInternal(ctx, &rt, false, false)
		}

		errs = multierror.Append(errs, err)
		for proxy, report := range itemProxyReports {
			// ok to return final proxy reports as the latest result includes latest proxy calculated
			// for each resource, as we process incrementally, storing new state in memory as we go
			proxyReports[proxy] = report
		}
	}

	if dryRun {
		v.latestSnapshot = &snap
	}

	return proxyReports, errs.ErrorOrNil()
}

func (v *validator) ValidateVirtualService(ctx context.Context, vs *v1.VirtualService, dryRun bool) (ProxyReports, error) {
	return v.validateVirtualServiceInternal(ctx, vs, dryRun, true)
}

func (v *validator) validateVirtualServiceInternal(ctx context.Context, vs *v1.VirtualService, dryRun, acquireLock bool) (ProxyReports, error) {
	apply := func(snap *v1.ApiSnapshot) ([]string, resources.Resource, core.ResourceRef) {
		vsRef := vs.GetMetadata().Ref()

		// TODO: move this to a function when generics become a thing
		var isUpdate bool
		for i, existingVs := range snap.VirtualServices {
			if vsRef == existingVs.GetMetadata().Ref() {
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

	return v.validateSnapshot(ctx, apply, dryRun, acquireLock)
}

func (v *validator) ValidateDeleteVirtualService(ctx context.Context, vsRef core.ResourceRef, dryRun bool) error {
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

	var parentGateways []core.ResourceRef
	snap.Gateways.Each(func(element *v1.Gateway) {
		http, ok := element.GatewayType.(*v1.Gateway_HttpGateway)
		if !ok {
			return
		}
		for _, ref := range http.HttpGateway.GetVirtualServices() {
			if ref == vsRef {
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
	apply := func(snap *v1.ApiSnapshot) ([]string, resources.Resource, core.ResourceRef) {
		rtRef := rt.GetMetadata().Ref()

		// TODO: move this to a function when generics become a thing
		var isUpdate bool
		for i, existingRt := range snap.RouteTables {
			if rtRef == existingRt.GetMetadata().Ref() {
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

	return v.validateSnapshot(ctx, apply, dryRun, acquireLock)
}

func (v *validator) ValidateDeleteRouteTable(ctx context.Context, rtRef core.ResourceRef, dryRun bool) error {
	if !v.ready() {
		return errors.Errorf("Gateway validation is yet not available. Waiting for first snapshot")
	}
	v.lock.Lock()
	snap := v.latestSnapshot.Clone()
	defer v.lock.Unlock()

	rt, err := snap.RouteTables.Find(rtRef.Strings())
	if err != nil {
		// if it's not present in the snapshot, allow deletion
		return nil
	}

	refsToDelete := refSet{rtRef: struct{}{}}

	var parentVirtualServices []core.ResourceRef
	snap.VirtualServices.Each(func(element *v1.VirtualService) {
		if routesContainRefs(element.GetVirtualHost().GetRoutes(), refsToDelete) {
			parentVirtualServices = append(parentVirtualServices, element.Metadata.Ref())
		}
	})

	var parentRouteTables []core.ResourceRef
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
	apply := func(snap *v1.ApiSnapshot) ([]string, resources.Resource, core.ResourceRef) {
		gwRef := gw.GetMetadata().Ref()

		// TODO: move this to a function when generics become a thing
		var isUpdate bool
		for i, existingGw := range snap.Gateways {
			if gwRef == existingGw.GetMetadata().Ref() {
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

	return v.validateSnapshot(ctx, apply, dryRun, acquireLock)
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

type refSet map[core.ResourceRef]struct{}

func virtualServicesForRouteTable(rt *v1.RouteTable, allVirtualServices v1.VirtualServiceList, allRoutes v1.RouteTableList) v1.VirtualServiceList {
	// this route table + its parents
	refsContainingRouteTable := refSet{rt.Metadata.Ref(): struct{}{}}

	// keep going until the ref list stops expanding
	for countedRefs := 0; countedRefs != len(refsContainingRouteTable); {
		countedRefs = len(refsContainingRouteTable)
		for _, rt := range allRoutes {
			if routesContainRefs(rt.GetRoutes(), refsContainingRouteTable) {
				refsContainingRouteTable[rt.Metadata.Ref()] = struct{}{}
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

		if _, ok := refs[*routeTableRef]; ok {
			return true
		}
	}
	return false
}

func gatewayListContainsVirtualService(gwList v1.GatewayList, vs *v1.VirtualService) bool {
	for _, gw := range gwList {
		if translator.GatewayContainsVirtualService(gw, vs) {
			return true
		}
	}

	return false
}
