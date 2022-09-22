package validation

import (
	"context"
	"sort"
	"sync"

	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/go-utils/hashutils"

	"github.com/hashicorp/go-multierror"
	errors "github.com/rotisserie/eris"
	utils2 "github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloo_translator "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	skprotoutils "github.com/solo-io/solo-kit/pkg/utils/protoutils"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

type Reports struct {
	Proxies         []*gloov1.Proxy
	ProxyReports    *ProxyReports
	UpstreamReports *UpstreamReports
}

func (r *Reports) GetProxies() []*gloov1.Proxy {
	if r == nil || r.Proxies == nil {
		return []*gloov1.Proxy{}
	}
	return r.Proxies
}

type ProxyReports []*validation.ProxyReport
type UpstreamReports []*validation.ResourceReport

var (
	NotReadyErr = errors.Errorf("validation is not yet available. Waiting for first snapshot")

	RouteTableDeleteErr = func(parentVirtualServices, parentRouteTables []*core.ResourceRef) error {
		return errors.Errorf("Deletion blocked because active Routes delegate to this Route Table. Remove delegate actions to this route table from the virtual services: %v and the route tables: %v, then try again",
			parentVirtualServices,
			parentRouteTables)
	}
	VirtualServiceDeleteErr = func(parentGateways []*core.ResourceRef) error {
		return errors.Errorf("Deletion blocked because active Gateways reference this Virtual Service. Remove refs to this virtual service from the gateways: %v, then try again",
			parentGateways)
	}
	unmarshalErrMsg     = "could not unmarshal raw object"
	WrappedUnmarshalErr = func(err error) error {
		return errors.Wrapf(err, unmarshalErrMsg)
	}

	GlooValidationResponseLengthError = func(resp *validation.GlooValidationServiceResponse) error {
		return errors.Errorf("Expected Gloo validation response to contain 1 report, but contained %d",
			len(resp.GetValidationReports()))
	}

	mValidConfig = utils2.MakeGauge("validation.gateway.solo.io/valid_config",
		"A boolean indicating whether gloo config is valid")
)

const (
	InvalidSnapshotErrMessage = "validation is disabled due to an invalid resource which has been written to storage. " +
		"Please correct any Rejected resources to re-enable validation."
)

var _ Validator = &validator{}

type Validator interface {
	gloov1snap.ApiSyncer
	ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (*Reports, *multierror.Error)
	ValidateGateway(ctx context.Context, gw *v1.Gateway, dryRun bool) (*Reports, error)
	ValidateVirtualService(ctx context.Context, vs *v1.VirtualService, dryRun bool) (*Reports, error)
	ValidateDeleteVirtualService(ctx context.Context, vs *core.ResourceRef, dryRun bool) error
	ValidateRouteTable(ctx context.Context, rt *v1.RouteTable, dryRun bool) (*Reports, error)
	ValidateDeleteRouteTable(ctx context.Context, rt *core.ResourceRef, dryRun bool) error
	ValidateUpstream(ctx context.Context, us *gloov1.Upstream, dryRun bool) (*Reports, error)
	ValidateDeleteUpstream(ctx context.Context, us *core.ResourceRef, dryRun bool) error
	ValidateDeleteSecret(ctx context.Context, secret *core.ResourceRef, dryRun bool) error
}

type ValidatorFunc = func(
	context.Context,
	*validation.GlooValidationServiceRequest,
) (*validation.GlooValidationServiceResponse, error)

type validator struct {
	lock              sync.RWMutex
	latestSnapshot    *gloov1snap.ApiSnapshot
	latestSnapshotErr error
	translator        translator.Translator
	//This function replaces a grpc client from when gloo and gateway pods were separate.
	validationFunc               ValidatorFunc
	ignoreProxyValidationFailure bool
	allowWarnings                bool
}

type ValidatorConfig struct {
	translator                   translator.Translator
	validatorFunc                ValidatorFunc
	writeNamespace               string
	ignoreProxyValidationFailure bool
	allowWarnings                bool
}

func NewValidatorConfig(
	translator translator.Translator,
	validatorFunc ValidatorFunc,
	ignoreProxyValidationFailure, allowWarnings bool,
) ValidatorConfig {
	return ValidatorConfig{
		translator:                   translator,
		validatorFunc:                validatorFunc,
		ignoreProxyValidationFailure: ignoreProxyValidationFailure,
		allowWarnings:                allowWarnings,
	}
}

func NewValidator(cfg ValidatorConfig) *validator {
	return &validator{
		translator:                   cfg.translator,
		validationFunc:               cfg.validatorFunc,
		ignoreProxyValidationFailure: cfg.ignoreProxyValidationFailure,
		allowWarnings:                cfg.allowWarnings,
	}
}

func (v *validator) ready() bool {
	return v.latestSnapshot != nil
}

func (v *validator) Sync(ctx context.Context, snap *gloov1snap.ApiSnapshot) error {
	v.lock.Lock() // hashing and cloning resources may mutate the object, so we need to lock
	defer v.lock.Unlock()
	if !v.gatewayUpdate(snap) {
		return nil
	}
	snapCopy := snap.Clone()
	gatewaysByProxy := utils.GatewaysByProxyName(snap.Gateways)
	var errs error
	for proxyName, gatewayList := range gatewaysByProxy {
		_, reports := v.translator.Translate(ctx, proxyName, snap, gatewayList)
		validate := reports.ValidateStrict
		if v.allowWarnings {
			validate = reports.Validate
		}
		if err := validate(); err != nil {
			errs = multierr.Append(errs, err)
		}
	}

	v.lock.Lock()
	defer v.lock.Unlock()

	// When the pod is first starting (aka the first snapshot is received),
	// set the value of mValidConfig with respect to the translation loop above.
	// Without this, mValidConfig will not be exported on /metrics until a new
	// resource is applied (https://github.com/solo-io/gloo/issues/5949).
	if v.latestSnapshot == nil {
		if errs == nil {
			utils2.MeasureOne(ctx, mValidConfig)
		} else {
			utils2.MeasureZero(ctx, mValidConfig)
		}
	}

	v.latestSnapshotErr = errs
	v.latestSnapshot = &snapCopy

	if errs != nil {
		return errors.Wrapf(errs, InvalidSnapshotErrMessage)
	}

	return nil
}

type applyResource func(snap *gloov1snap.ApiSnapshot) (proxyNames []string, resource resources.Resource, ref *core.ResourceRef)

func (v *validator) gatewayUpdate(snap *gloov1snap.ApiSnapshot) bool {

	if v.latestSnapshot == nil {
		return true
	}
	//look at the hash of resources that affect the gateway snapshot
	hashFunc := func(snap *gloov1snap.ApiSnapshot) uint64 {
		toHash := append([]interface{}{}, snap.VirtualHostOptions.AsInterfaces()...)
		toHash = append(toHash, snap.VirtualServices.AsInterfaces()...)
		toHash = append(toHash, snap.Gateways.AsInterfaces()...)
		toHash = append(toHash, snap.RouteOptions.AsInterfaces()...)
		toHash = append(toHash, snap.RouteTables.AsInterfaces()...)
		toHash = append(toHash, snap.HttpGateways.AsInterfaces()...)
		toHash = append(toHash, snap.VirtualHostOptions.AsInterfaces()...)
		hash, err := hashutils.HashAllSafe(nil, toHash...)
		if err != nil {
			panic("this error should never happen, as this is safe hasher")
		}
		return hash
	}
	hashChanged := hashFunc(v.latestSnapshot) != hashFunc(snap)
	return hashChanged
}

// update internal snapshot to handle race where a lot of resources may be deleted at once, before syncer updates
// should be called within a lock
func (v *validator) deleteFromLocalSnapshot(resource resources.Resource) {
	ref := resource.GetMetadata().Ref()
	switch resource.(type) {
	case *v1.VirtualService:
		for i, rt := range v.latestSnapshot.VirtualServices {
			if rt.GetMetadata().Ref().Equal(ref) {
				v.latestSnapshot.VirtualServices = append(v.latestSnapshot.VirtualServices[:i],
					v.latestSnapshot.VirtualServices[i+1:]...)
				break
			}
		}
	case *v1.RouteTable:
		for i, rt := range v.latestSnapshot.RouteTables {
			if rt.GetMetadata().Ref().Equal(ref) {
				v.latestSnapshot.RouteTables = append(v.latestSnapshot.RouteTables[:i], v.latestSnapshot.RouteTables[i+1:]...)
				break
			}
		}
	}
}

func (v *validator) validateSnapshotThreadSafe(ctx context.Context, apply applyResource, dryRun bool) (
	*Reports,
	error,
) {
	// thread-safe implementation of validateSnapshot

	v.lock.Lock()
	defer v.lock.Unlock()

	return v.validateSnapshot(ctx, apply, dryRun)
}

func (v *validator) validateSnapshot(ctx context.Context, apply applyResource, dryRun bool) (*Reports, error) {
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
		if !dryRun {
			utils2.MeasureZero(ctx, mValidConfig)
		}
		contextutils.LoggerFrom(ctx).Errorw(InvalidSnapshotErrMessage, zap.Error(v.latestSnapshotErr))
		// allow writes if storage is already broken
		return nil, nil
	}

	// verify the mutation against a snapshot clone first, only apply the change to the actual snapshot if this passes
	proxyNames, resource, ref := apply(&snapshotClone)

	gatewaysByProxy := utils.GatewaysByProxyName(snapshotClone.Gateways)

	var (
		errs         error
		proxyReports ProxyReports
		proxies      []*gloov1.Proxy
	)
	for _, proxyName := range proxyNames {
		gatewayList := gatewaysByProxy[proxyName]
		proxy, reports := v.translator.Translate(ctx, proxyName, &snapshotClone, gatewayList)
		validate := reports.ValidateStrict
		if v.allowWarnings {
			validate = reports.Validate
		}
		if err := validate(); err != nil {
			errs = multierr.Append(errs, errors.Wrapf(err, "could not render proxy"))
			continue
		}

		// a nil proxy may have been returned if 0 listeners were created
		if proxy == nil {
			continue
		}

		proxies = append(proxies, proxy)
		// validate the proxy with gloo
		glooValidationResponse, err := v.sendGlooValidationServiceRequest(ctx, &validation.GlooValidationServiceRequest{
			Proxy: proxy,
		})
		if err != nil {
			err = errors.Wrapf(err, "failed to communicate with Gloo validation server")
			if v.ignoreProxyValidationFailure {
				contextutils.LoggerFrom(ctx).Error(err)
			} else {
				errs = multierr.Append(errs, err)
			}
			continue
		}
		if len(glooValidationResponse.GetValidationReports()) != 1 {
			// This was likely caused by a development error
			err := GlooValidationResponseLengthError(glooValidationResponse)
			errs = multierr.Append(errs, err)
			continue
		}

		proxyReport := glooValidationResponse.GetValidationReports()[0].GetProxyReport()
		proxyReports = append(proxyReports, proxyReport)
		if err := validationutils.GetProxyError(proxyReport); err != nil {
			errs = multierr.Append(errs, errors.Wrapf(err, "failed to validate Proxy with Gloo validation server"))
			continue
		}
		if warnings := validationutils.GetProxyWarning(proxyReport); !v.allowWarnings && len(warnings) > 0 {
			for _, warning := range warnings {
				errs = multierr.Append(errs, errors.New(warning))
			}
			continue
		}
	}

	if errs != nil {
		contextutils.LoggerFrom(ctx).Debugf("Rejected %T %v: %v", resource, ref, errs)
		if !dryRun {
			utils2.MeasureZero(ctx, mValidConfig)
		}
		return &Reports{ProxyReports: &proxyReports, Proxies: proxies}, errors.Wrapf(errs,
			"validating %T %v",
			resource,
			ref)
	}

	contextutils.LoggerFrom(ctx).Debugf("Accepted %T %v", resource, ref)
	if !dryRun {
		utils2.MeasureOne(ctx, mValidConfig)
	}

	if !dryRun {
		// update internal snapshot to handle race where a lot of resources may be applied at once, before syncer updates
		apply(v.latestSnapshot)
	}

	return &Reports{ProxyReports: &proxyReports, Proxies: proxies}, nil
}

func (v *validator) ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (
	*Reports,
	*multierror.Error,
) {
	var (
		proxies      []*gloov1.Proxy
		proxyReports = ProxyReports{}
		errs         = &multierror.Error{}
	)

	v.lock.Lock()
	defer v.lock.Unlock()
	originalSnapshot := v.latestSnapshot.Clone()
	for _, item := range ul.Items {

		var itemProxyReports, err = v.processItem(ctx, item)

		errs = multierror.Append(errs, err)
		if itemProxyReports != nil && itemProxyReports.ProxyReports != nil {
			for _, report := range *itemProxyReports.ProxyReports {
				// ok to return final proxy reports as the latest result includes latest proxy calculated
				// for each resource, as we process incrementally, storing new state in memory as we go
				proxyReports = append(proxyReports, report)
			}
			for _, proxy := range itemProxyReports.Proxies {
				proxies = append(proxies, proxy)
			}
		}
	}

	if dryRun {
		// to validate the entire list of changes against one another, each item was applied to the latestSnapshot
		// if this is a dry run, latestSnapshot needs to be reset back to its original value without any of the changes
		v.latestSnapshot = &originalSnapshot
	}

	return &Reports{ProxyReports: &proxyReports, Proxies: proxies}, errs
}

func (v *validator) processItem(ctx context.Context, item unstructured.Unstructured) (*Reports, error) {
	// process a single change in a list of changes
	//
	// when calling the specific internal validate method, dryRun and acquireLock are always false:
	// 	dryRun=false: this enables items to be validated against other items in the list
	// 	acquireLock=false: the entire list of changes are called within a single lock
	gv, err := schema.ParseGroupVersion(item.GetAPIVersion())
	if err != nil {
		return &Reports{ProxyReports: &ProxyReports{}}, err
	}

	itemGvk := schema.GroupVersionKind{
		Version: gv.Version,
		Group:   gv.Group,
		Kind:    item.GetKind(),
	}

	jsonBytes, err := item.MarshalJSON()
	if err != nil {
		return &Reports{ProxyReports: &ProxyReports{}}, err
	}

	switch itemGvk {
	case v1.GatewayGVK:
		var (
			gw v1.Gateway
		)
		if unmarshalErr := skprotoutils.UnmarshalResource(jsonBytes, &gw); unmarshalErr != nil {
			return &Reports{ProxyReports: &ProxyReports{}}, WrappedUnmarshalErr(unmarshalErr)
		}
		return v.validateGatewayInternal(ctx, &gw, false, false)
	case v1.VirtualServiceGVK:
		var (
			vs v1.VirtualService
		)
		if unmarshalErr := skprotoutils.UnmarshalResource(jsonBytes, &vs); unmarshalErr != nil {
			return &Reports{ProxyReports: &ProxyReports{}}, WrappedUnmarshalErr(unmarshalErr)
		}
		return v.validateVirtualServiceInternal(ctx, &vs, false, false)
	case v1.RouteTableGVK:
		var (
			rt v1.RouteTable
		)
		if unmarshalErr := skprotoutils.UnmarshalResource(jsonBytes, &rt); unmarshalErr != nil {
			return &Reports{ProxyReports: &ProxyReports{}}, WrappedUnmarshalErr(unmarshalErr)
		}
		return v.validateRouteTableInternal(ctx, &rt, false, false)

	case gloov1.UpstreamGVK:
		// TODO(mitchaman): Handle upstreams
	}
	// should not happen
	return &Reports{ProxyReports: &ProxyReports{}}, errors.Errorf("Unknown group/version/kind, %v", itemGvk)
}

func (v *validator) ValidateVirtualService(ctx context.Context, vs *v1.VirtualService, dryRun bool) (*Reports, error) {
	return v.validateVirtualServiceInternal(ctx, vs, dryRun, true)
}

func (v *validator) validateVirtualServiceInternal(
	ctx context.Context,
	vs *v1.VirtualService,
	dryRun, acquireLock bool,
) (*Reports, error) {
	apply := func(snap *gloov1snap.ApiSnapshot) ([]string, resources.Resource, *core.ResourceRef) {
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

		return proxiesForVirtualService(ctx, snap.Gateways, snap.HttpGateways, vs), vs, vsRef
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
		virtualServices := virtualServicesForGateway(ctx, snap, element)
		if len(virtualServices) == 0 {
			contextutils.LoggerFrom(ctx).Debugw("Accepted deletion of Virtual Service %v", vsRef)
			return
		}

		for _, ref := range virtualServices {
			if ref.Equal(vsRef) {
				// this gateway points at this virtual service
				parentGateways = append(parentGateways, element.GetMetadata().Ref())

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

func (v *validator) ValidateRouteTable(ctx context.Context, rt *v1.RouteTable, dryRun bool) (*Reports, error) {
	return v.validateRouteTableInternal(ctx, rt, dryRun, true)
}

func (v *validator) validateRouteTableInternal(
	ctx context.Context,
	rt *v1.RouteTable,
	dryRun, acquireLock bool,
) (*Reports, error) {
	apply := func(snap *gloov1snap.ApiSnapshot) ([]string, resources.Resource, *core.ResourceRef) {
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

		proxiesToConsider := proxiesForRouteTable(ctx, snap, rt)

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

	refsToDelete := sets.NewString(gloo_translator.UpstreamToClusterName(rtRef))

	var parentVirtualServices []*core.ResourceRef
	snap.VirtualServices.Each(func(element *v1.VirtualService) {
		// for each VS, check if its routes contain a ref to deleted rt
		if routesContainRefs(element.GetVirtualHost().GetRoutes(), refsToDelete) {
			parentVirtualServices = append(parentVirtualServices, element.GetMetadata().Ref())
		}
	})

	var parentRouteTables []*core.ResourceRef
	snap.RouteTables.Each(func(element *v1.RouteTable) {
		// for each RT, check if its routes contain a ref to deleted rt
		if routesContainRefs(element.GetRoutes(), refsToDelete) {
			parentRouteTables = append(parentRouteTables, element.GetMetadata().Ref())
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

func (v *validator) ValidateGateway(ctx context.Context, gw *v1.Gateway, dryRun bool) (*Reports, error) {
	return v.validateGatewayInternal(ctx, gw, dryRun, true)
}

func (v *validator) validateGatewayInternal(ctx context.Context, gw *v1.Gateway, dryRun, acquireLock bool) (
	*Reports,
	error,
) {
	apply := func(snap *gloov1snap.ApiSnapshot) ([]string, resources.Resource, *core.ResourceRef) {
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

func (v *validator) ValidateUpstream(ctx context.Context, us *gloov1.Upstream, dryRun bool) (*Reports, error) {
	response, err := v.sendGlooValidationServiceRequest(ctx, &validation.GlooValidationServiceRequest{
		// Sending a nil proxy causes the upstream to be translated with all proxies in gloo's snapshot
		Proxy: nil,
		Resources: &validation.GlooValidationServiceRequest_ModifiedResources{
			ModifiedResources: &validation.ModifiedResources{
				Upstreams: []*gloov1.Upstream{us},
			},
		},
	})
	logger := contextutils.LoggerFrom(ctx)
	if err != nil {
		if v.ignoreProxyValidationFailure {
			logger.Error(err)
		} else {
			return &Reports{}, err
		}
	}
	// dont log the responsse as proxies and status reports may be too large in large envs.
	logger.Debugf("Got response from GlooValidationService with %d reports", len(response.GetValidationReports()))

	return v.getReportsFromGlooValidationResponse(response)
}

func (v *validator) ValidateDeleteUpstream(ctx context.Context, upstreamRef *core.ResourceRef, dryRun bool) error {
	response, err := v.sendGlooValidationServiceRequest(ctx, &validation.GlooValidationServiceRequest{
		// Sending a nil proxy causes the remaining upstreams to be translated with all proxies in gloo's snapshot
		Proxy: nil,
		Resources: &validation.GlooValidationServiceRequest_DeletedResources{
			DeletedResources: &validation.DeletedResources{
				UpstreamRefs: []*core.ResourceRef{upstreamRef},
			},
		},
	})
	logger := contextutils.LoggerFrom(ctx)
	if err != nil {
		if v.ignoreProxyValidationFailure {
			logger.Error(err)
		} else {
			return err
		}
	}

	// dont log the responsse as proxies and status reports may be too large in large envs.
	logger.Debugf("Got response from GlooValidationService with %d reports", len(response.GetValidationReports()))

	_, err = v.getReportsFromGlooValidationResponse(response)

	return err
}

func (v *validator) ValidateDeleteSecret(ctx context.Context, secretRef *core.ResourceRef, dryRun bool) error {
	response, err := v.sendGlooValidationServiceRequest(ctx, &validation.GlooValidationServiceRequest{
		// Sending a nil proxy causes the remaining secrets to be translated with all proxies in gloo's snapshot
		Proxy: nil,
		Resources: &validation.GlooValidationServiceRequest_DeletedResources{
			DeletedResources: &validation.DeletedResources{
				SecretRefs: []*core.ResourceRef{secretRef},
			},
		},
	})
	logger := contextutils.LoggerFrom(ctx)
	if err != nil {
		if v.ignoreProxyValidationFailure {
			logger.Error(err)
		} else {
			return err
		}
	}

	// dont log the responsse as proxies and status reports may be too large in large envs.
	logger.Debugf("Got response from GlooValidationService with %d reports", len(response.GetValidationReports()))

	_, err = v.getReportsFromGlooValidationResponse(response)
	return err
}

// Converts the GlooValidationServiceResponse into Reports.
func (v *validator) getReportsFromGlooValidationResponse(validationResponse *validation.GlooValidationServiceResponse) (
	*Reports,
	error,
) {
	var (
		errs            error
		upstreamReports UpstreamReports
		proxyReports    ProxyReports
		proxies         []*gloov1.Proxy
	)
	for _, report := range validationResponse.GetValidationReports() {
		// Append upstream errors
		for _, usRpt := range report.GetUpstreamReports() {
			upstreamReports = append(upstreamReports, usRpt)
			if err := resourceReportToMultiErr(usRpt); err != nil {
				errs = multierr.Append(errs, errors.Wrapf(err, "failed to validate Upstream with Gloo validation server"))
			}
			if warnings := usRpt.GetWarnings(); !v.allowWarnings && len(warnings) > 0 {
				for _, warning := range warnings {
					errs = multierr.Append(errs, errors.New(warning))
				}
			}
		}

		// Append proxies and proxy reports
		if report.GetProxy() != nil {
			proxies = append(proxies, report.GetProxy())
		}
		if proxyReport := report.GetProxyReport(); proxyReport != nil {
			proxyReports = append(proxyReports, report.GetProxyReport())
			if err := validationutils.GetProxyError(proxyReport); err != nil {
				errs = multierr.Append(errs, errors.Wrapf(err, "failed to validate Proxy with Gloo validation server"))
			}
			if warnings := validationutils.GetProxyWarning(proxyReport); !v.allowWarnings && len(warnings) > 0 {
				for _, warning := range warnings {
					errs = multierr.Append(errs, errors.New(warning))
				}
			}
		}
	}
	return &Reports{
		ProxyReports:    &proxyReports,
		UpstreamReports: &upstreamReports,
		Proxies:         proxies,
	}, errs
}

func (v *validator) sendGlooValidationServiceRequest(
	ctx context.Context,
	req *validation.GlooValidationServiceRequest,
) (*validation.GlooValidationServiceResponse, error) {
	logger := contextutils.LoggerFrom(ctx)
	logger.Debugf("Sending request validation request modified:%s, deleted:%s",
		req.GetModifiedResources().String(), req.GetDeletedResources().String())

	return v.validationFunc(ctx, req)
}

func proxiesForVirtualService(ctx context.Context, gwList v1.GatewayList, httpGwList v1.MatchableHttpGatewayList, vs *v1.VirtualService) []string {
	gatewaysByProxy := utils.GatewaysByProxyName(gwList)

	var proxiesToConsider []string

	for proxyName, gatewayList := range gatewaysByProxy {
		if gatewayListContainsVirtualService(ctx, gatewayList, httpGwList, vs) {
			// we only care about validating this proxy if it contains this virtual service
			proxiesToConsider = append(proxiesToConsider, proxyName)
		}
	}

	sort.Strings(proxiesToConsider)

	return proxiesToConsider
}

func virtualServicesForGateway(ctx context.Context, snap gloov1snap.ApiSnapshot, gateway *v1.Gateway) []*core.ResourceRef {
	var virtualServices []*core.ResourceRef

	switch gatewayType := gateway.GetGatewayType().(type) {
	case *v1.Gateway_TcpGateway:
		// TcpGateway does not configure VirtualServices
		break
	case *v1.Gateway_HttpGateway:
		virtualServices = gatewayType.HttpGateway.GetVirtualServices()
	case *v1.Gateway_HybridGateway:
		matchedGateways := gatewayType.HybridGateway.GetMatchedGateways()
		if matchedGateways != nil {
			for _, matchedGateway := range matchedGateways {
				if httpGateway := matchedGateway.GetHttpGateway(); httpGateway != nil {
					virtualServices = append(virtualServices, httpGateway.GetVirtualServices()...)
				}
			}
		} else {
			delegatedGateways := gatewayType.HybridGateway.GetDelegatedHttpGateways()
			httpGatewayList := translator.NewHttpGatewaySelector(snap.HttpGateways).SelectMatchableHttpGateways(delegatedGateways, func(err error) {
				logger := contextutils.LoggerFrom(ctx)
				logger.Warnf("failed to select matchable http gateways on gateway: %v", err.Error())
			})

			httpGatewayList.Each(func(element *v1.MatchableHttpGateway) {
				virtualServices = append(virtualServices, element.GetHttpGateway().GetVirtualServices()...)
			})
		}
	}

	return virtualServices
}

func proxiesForRouteTable(ctx context.Context, snap *gloov1snap.ApiSnapshot, rt *v1.RouteTable) []string {
	affectedVirtualServices := virtualServicesForRouteTable(rt, snap.VirtualServices, snap.RouteTables)

	affectedProxies := make(map[string]struct{})
	for _, vs := range affectedVirtualServices {
		proxiesToConsider := proxiesForVirtualService(ctx, snap.Gateways, snap.HttpGateways, vs)
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

type routeTableSet map[string]*v1.RouteTable

// gets all the virtual services that have the given route table as a descendent via delegation
func virtualServicesForRouteTable(
	rt *v1.RouteTable,
	allVirtualServices v1.VirtualServiceList,
	allRouteTables v1.RouteTableList,
) v1.VirtualServiceList {
	// To determine all the virtual services that delegate to this route table (either directly or via a delegate
	// chain), we first find all the ancestor route tables that are part of a delegate chain leading to this route
	// table, and then find all the virtual services that delegate (via ref or selector) to any of those routes.

	// build up a set of route tables including this route table and its ancestors
	relevantRouteTables := routeTableSet{gloo_translator.UpstreamToClusterName(rt.GetMetadata().Ref()): rt}

	// keep going until the ref list stops expanding
	for countedRefs := 0; countedRefs != len(relevantRouteTables); {
		countedRefs = len(relevantRouteTables)
		for _, candidateRt := range allRouteTables {
			// for each RT, if it delegates to any of the relevant RTs, add it to the set of relevant RTs
			if routesContainSelectorsOrRefs(candidateRt.GetRoutes(),
				candidateRt.GetMetadata().GetNamespace(),
				relevantRouteTables) {
				relevantRouteTables[gloo_translator.UpstreamToClusterName(candidateRt.GetMetadata().Ref())] = candidateRt
			}
		}
	}

	var parentVirtualServices v1.VirtualServiceList
	for _, candidateVs := range allVirtualServices {
		// for each VS, check if its routes delegate to any of the relevant RTs
		if routesContainSelectorsOrRefs(candidateVs.GetVirtualHost().GetRoutes(),
			candidateVs.GetMetadata().GetNamespace(),
			relevantRouteTables) {
			parentVirtualServices = append(parentVirtualServices, candidateVs)
		}
	}

	return parentVirtualServices
}

// Returns true if any of the given routes delegate to any of the given route tables via a direct reference.
// This is used to determine which route tables are affected when a route table is deleted. Since selectors do not
// represent hard referential constraints, we only need to check direct references here (we can safely remove a route
// table that matches via a selector).
func routesContainRefs(routes []*v1.Route, refs sets.String) bool {
	for _, r := range routes {
		delegate := r.GetDelegateAction()
		if delegate == nil {
			continue
		}

		rtRef := getDelegateRef(delegate)
		if rtRef == nil {
			continue
		}

		if _, ok := refs[gloo_translator.UpstreamToClusterName(rtRef)]; ok {
			return true
		}
	}
	return false
}

// Returns true if any of the given routes delegate to any of the given route tables via either a direct reference
// or a selector. This is used to determine which route tables are affected when a route table is added/modified.
func routesContainSelectorsOrRefs(routes []*v1.Route, parentNamespace string, routeTables routeTableSet) bool {
	// convert to list for passing into translator func
	rtList := make([]*v1.RouteTable, 0, len(routeTables))
	for _, rt := range routeTables {
		rtList = append(rtList, rt)
	}

	for _, r := range routes {
		delegate := r.GetDelegateAction()
		if delegate == nil {
			continue
		}

		// check if this route delegates to any of the given route tables via ref
		rtRef := getDelegateRef(delegate)
		if rtRef != nil {
			if _, ok := routeTables[gloo_translator.UpstreamToClusterName(rtRef)]; ok {
				return true
			}
			continue
		}

		// check if this route delegates to any of the given route tables via selector
		rtSelector := delegate.GetSelector()
		if rtSelector != nil {
			// this will return the subset of the RT list that matches the selector
			selectedRtList, err := translator.RouteTablesForSelector(rtList, rtSelector, parentNamespace)
			if err != nil {
				return false
			}

			if len(selectedRtList) > 0 {
				return true
			}
		}
	}
	return false
}

func getDelegateRef(delegate *v1.DelegateAction) *core.ResourceRef {
	// handle deprecated route table resource reference format
	// TODO(marco): remove when we remove the deprecated fields from the API
	if delegate.GetNamespace() != "" || delegate.GetName() != "" {
		return &core.ResourceRef{
			Namespace: delegate.GetNamespace(),
			Name:      delegate.GetName(),
		}
	} else if delegate.GetRef() != nil {
		return delegate.GetRef()
	}
	return nil
}

func gatewayListContainsVirtualService(ctx context.Context, gwList v1.GatewayList, httpGwList v1.MatchableHttpGatewayList, vs *v1.VirtualService) bool {
	for _, gw := range gwList {
		if gatewayContainsVirtualService(ctx, httpGwList, gw, vs) {
			return true
		}
	}

	return false
}

func gatewayContainsVirtualService(ctx context.Context, httpGwList v1.MatchableHttpGatewayList, gw *v1.Gateway, vs *v1.VirtualService) bool {
	if gw.GetTcpGateway() != nil {
		return false
	}

	if httpGateway := gw.GetHttpGateway(); httpGateway != nil {
		return httpGatewayContainsVirtualService(httpGateway, vs, gw.GetSsl())
	}

	if hybridGateway := gw.GetHybridGateway(); hybridGateway != nil {
		matchedGateways := hybridGateway.GetMatchedGateways()
		if matchedGateways != nil {
			for _, mg := range hybridGateway.GetMatchedGateways() {
				if httpGateway := mg.GetHttpGateway(); httpGateway != nil {
					if httpGatewayContainsVirtualService(httpGateway, vs, mg.GetMatcher().GetSslConfig() != nil) {
						return true
					}
				}
			}
		} else {
			delegatedGateway := hybridGateway.GetDelegatedHttpGateways()
			selectedGatewayList := translator.NewHttpGatewaySelector(httpGwList).SelectMatchableHttpGateways(delegatedGateway, func(err error) {
				logger := contextutils.LoggerFrom(ctx)
				logger.Warnf("failed to select matchable http gateways on gateway: %v", err.Error())
			})
			for _, selectedHttpGw := range selectedGatewayList {
				if httpGatewayContainsVirtualService(selectedHttpGw.GetHttpGateway(), vs, selectedHttpGw.GetMatcher().GetSslConfig() != nil) {
					return true
				}
			}
		}

	}

	return false
}

func httpGatewayContainsVirtualService(httpGateway *v1.HttpGateway, vs *v1.VirtualService, requireSsl bool) bool {
	contains, err := translator.HttpGatewayContainsVirtualService(httpGateway, vs, requireSsl)
	if err != nil {
		return false
	}
	return contains
}

func resourceReportToMultiErr(resourceRpt *validation.ResourceReport) error {
	var multiErr error
	for _, errStr := range resourceRpt.GetErrors() {
		multiErr = multierr.Append(multiErr, errors.New(errStr))
	}
	return multiErr
}
