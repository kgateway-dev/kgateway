package validation

import (
	"context"
	"sort"
	"sync"

	"go.uber.org/multierr"

	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
)

var (
	NotReadyErr = errors.Errorf("validation is yet not available. Waiting for first snapshot")

	RouteTableDeleteErr = func(parentVirtualServices, parentRouteTables []core.ResourceRef) error {
		return errors.Errorf("Deletion blocked because active Routes delegate to this Route Table. Remove delegate actions to this route table the virtual services: %v and the route tables: %v, then try again", parentVirtualServices, parentRouteTables)
	}
	VirtualServiceDeleteErr = func(parentGateways []core.ResourceRef) error {
		return errors.Errorf("Deletion blocked because active Gateways reference this Virtual Service. Remove refs to this virtual service from the gateways: %v, then try again", parentGateways)
	}
)

const (
	InvalidSnapshotErrMessage = "validation is disabled due to an invalid resource which has been written to storage. " +
		"Please correct any Rejected resources to re-enable validation."
)

type Validator interface {
	v2.ApiSyncer
	Ready() bool
	ValidateGateway(ctx context.Context, gw *v2.Gateway) error
	ValidateVirtualService(ctx context.Context, vs *v1.VirtualService) error
	ValidateDeleteVirtualService(ctx context.Context, vs core.ResourceRef) error
	ValidateRouteTable(ctx context.Context, rt *v1.RouteTable) error
	ValidateDeleteRouteTable(ctx context.Context, rt core.ResourceRef) error
}

type validator struct {
	l                 sync.RWMutex
	latestSnapshot    *v2.ApiSnapshot
	latestSnapshotErr error
	translator        translator.Translator
	validationClient  validation.ProxyValidationServiceClient
	writeNamespace    string
}

func NewValidator(translator translator.Translator, validationClient validation.ProxyValidationServiceClient, writeNamespace string) *validator {
	return &validator{translator: translator, validationClient: validationClient, writeNamespace: writeNamespace}
}

func (v *validator) Ready() bool {
	return v.latestSnapshot != nil
}

func (v *validator) Sync(ctx context.Context, snap *v2.ApiSnapshot) error {
	snapCopy := snap.Clone()
	gatewaysByProxy := utils.GatewaysByProxyName(snap.Gateways)
	var errs error
	for proxyName := range gatewaysByProxy {
		gatewayList := gatewaysByProxy[proxyName]
		_, resourceErrs := v.translator.Translate(ctx, proxyName, v.writeNamespace, snap, gatewayList)
		if err := resourceErrs.Validate(); err != nil {
			errs = multierr.Append(errs, err)
		}
	}

	v.l.Lock()
	defer v.l.Unlock()

	v.latestSnapshotErr = errs
	v.latestSnapshot = &snapCopy

	if errs != nil {
		return errors.Wrapf(errs, InvalidSnapshotErrMessage)
	}

	return nil
}

func (v *validator) validateSnapshot(ctx context.Context, snap *v2.ApiSnapshot, proxyNames []string) error {
	if !v.Ready() {
		return NotReadyErr
	}

	if v.latestSnapshotErr != nil {
		contextutils.LoggerFrom(ctx).Errorw(InvalidSnapshotErrMessage, zap.Error(v.latestSnapshotErr))
		// allow writes if storage is already broken
		return nil
	}

	gatewaysByProxy := utils.GatewaysByProxyName(snap.Gateways)

	for _, proxyName := range proxyNames {
		gatewayList := gatewaysByProxy[proxyName]
		proxy, resourceErrs := v.translator.Translate(ctx, proxyName, v.writeNamespace, snap, gatewayList)
		if err := resourceErrs.Validate(); err != nil {
			return errors.Wrapf(err, "could not render proxy")
		}

		if v.validationClient == nil {
			contextutils.LoggerFrom(ctx).Warnf("skipping proxy validation check as the " +
				"Proxy validation client has not been initialized. check to ensure that the gateway and gloo processes " +
				"are configured to communicate.")
			return nil
		}

		// validate the proxy with gloo
		proxyReport, err := v.validationClient.ValidateProxy(ctx, &validation.ProxyValidationServiceRequest{Proxy: proxy})
		if err != nil {
			contextutils.LoggerFrom(ctx).Errorw("failed to validate Proxy with Gloo validation server.", zap.Error(err))
			return errors.Wrapf(err, "failed to validate Proxy with Gloo validation server")
		}

		if proxyErr := validationutils.GetProxyError(proxyReport.ProxyReport); proxyErr != nil {
			return errors.Wrapf(proxyErr, "rendered proxy had errors")
		}
	}
	return nil
}

func (v *validator) ValidateVirtualService(ctx context.Context, vs *v1.VirtualService) error {
	if !v.Ready() {
		return errors.Errorf("Gateway validation is yet not available. Waiting for first snapshot")
	}
	v.l.RLock()
	snap := v.latestSnapshot.Clone()
	v.l.RUnlock()

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

	proxiesToConsider := proxiesForVirtualService(snap.Gateways, vs)

	if err := v.validateSnapshot(ctx, &snap, proxiesToConsider); err != nil {
		contextutils.LoggerFrom(ctx).Debugw("Rejected %T %v: %v", vs, vsRef, err)
		return errors.Wrapf(err, "validating %T %v", vs, vsRef)
	}

	contextutils.LoggerFrom(ctx).Debugw("Accepted %T %v", vs, vsRef)

	return nil
}

func (v *validator) ValidateDeleteVirtualService(ctx context.Context, vsRef core.ResourceRef) error {
	if !v.Ready() {
		return errors.Errorf("Gateway validation is yet not available. Waiting for first snapshot")
	}
	v.l.RLock()
	snap := v.latestSnapshot.Clone()
	v.l.RUnlock()

	_, err := snap.VirtualServices.Find(vsRef.Strings())
	if err != nil {
		// if it's not present in the snapshot, allow deletion
		return nil
	}

	var parentGateways []core.ResourceRef
	snap.Gateways.Each(func(element *v2.Gateway) {
		http, ok := element.GatewayType.(*v2.Gateway_HttpGateway)
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
		contextutils.LoggerFrom(ctx).Debugw("Rejected deletion of Virtual Service %v: %v", vsRef, err)
		return err
	}

	contextutils.LoggerFrom(ctx).Debugw("Accepted deletion of Virtual Service %v", vsRef)

	return nil
}

func (v *validator) ValidateRouteTable(ctx context.Context, rt *v1.RouteTable) error {
	if !v.Ready() {
		return errors.Errorf("Gateway validation is yet not available. Waiting for first snapshot")
	}
	v.l.RLock()
	snap := v.latestSnapshot.Clone()
	v.l.RUnlock()

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

	if err := v.validateSnapshot(ctx, &snap, proxiesToConsider); err != nil {
		contextutils.LoggerFrom(ctx).Debugw("Rejected %T %v: %v", rt, rtRef, err)
		return errors.Wrapf(err, "validating %T %v", rt, rtRef)
	}

	contextutils.LoggerFrom(ctx).Debugw("Accepted %T %v", rt, rtRef)

	return nil
}

func (v *validator) ValidateDeleteRouteTable(ctx context.Context, rtRef core.ResourceRef) error {
	if !v.Ready() {
		return errors.Errorf("Gateway validation is yet not available. Waiting for first snapshot")
	}
	v.l.RLock()
	snap := v.latestSnapshot.Clone()
	v.l.RUnlock()

	_, err := snap.RouteTables.Find(rtRef.Strings())
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
		contextutils.LoggerFrom(ctx).Debugw("Rejected deletion of Route Table %v: %v", rtRef, err)
		return err
	}

	contextutils.LoggerFrom(ctx).Debugw("Accepted Route Table deletion %v", rtRef)

	return nil
}

func (v *validator) ValidateGateway(ctx context.Context, gw *v2.Gateway) error {
	if !v.Ready() {
		return errors.Errorf("Gateway validation is yet not available. Waiting for first snapshot")
	}
	v.l.RLock()
	snap := v.latestSnapshot.Clone()
	v.l.RUnlock()

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

	if err := v.validateSnapshot(ctx, &snap, proxiesToConsider); err != nil {
		contextutils.LoggerFrom(ctx).Debugw("Rejected %T %v: %v", gw, gwRef, err)
		return errors.Wrapf(err, "validating %T %v", gw, gwRef)
	}

	contextutils.LoggerFrom(ctx).Debugw("Accepted %T %v", gw, gwRef)

	return nil
}

func proxiesForVirtualService(gwList v2.GatewayList, vs *v1.VirtualService) []string {

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

func proxiesForRouteTable(gwList v2.GatewayList, vsList v1.VirtualServiceList, rtList v1.RouteTableList, rt *v1.RouteTable) []string {
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
		if _, ok := refs[*delegate]; ok {
			return true
		}
	}
	return false
}

func gatewayListContainsVirtualService(gwList v2.GatewayList, vs *v1.VirtualService) bool {
	for _, gw := range gwList {
		if translator.GatewayContainsVirtualService(gw, vs) {
			return true
		}
	}

	return false
}
