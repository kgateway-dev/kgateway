package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/go-utils/hashutils"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	errors "github.com/rotisserie/eris"
	utils2 "github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	syncerValidation "github.com/solo-io/gloo/projects/gloo/pkg/syncer/validation"
	validationutils "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
	gloovalidation "github.com/solo-io/gloo/projects/gloo/pkg/validation"
	"github.com/solo-io/go-utils/contextutils"
	kubeCRDV1 "github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/solo.io/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	skProtoUtils "github.com/solo-io/solo-kit/pkg/utils/protoutils"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const GatewayGroup = "gateway.solo.io"

type Reports struct {
	Proxies      []*gloov1.Proxy
	ProxyReports *ProxyReports
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
	NotReadyErr                    = errors.Errorf("validation is not yet available. Waiting for first snapshot")
	HasNotReceivedFirstSync        = eris.New("proxy validation called before the validation server received its first sync of resources")
	unmarshalErrMsg                = "could not unmarshal raw object"
	couldNotRenderProxy            = "could not render proxy"
	failedGlooValidation           = "failed gloo validation"
	failedResourceReports          = "failed gloo validation resource reports"
	failedExtensionResourceReports = "failed extension resource reports"
	WrappedUnmarshalErr            = func(err error) error {
		return errors.Wrapf(err, unmarshalErrMsg)
	}

	GlooValidationResponseLengthError = func(reports []*gloovalidation.GlooValidationReport) error {
		return errors.Errorf("Expected Gloo validation response to contain 1 report, but contained %d",
			len(reports))
	}

	proxyFailedGlooValidation = func(err error, proxy *gloov1.Proxy) error {
		return errors.Wrapf(err, "failed to validate Proxy [namespace: %s, name: %s] with gloo validation", proxy.GetMetadata().GetNamespace(), proxy.GetMetadata().GetName())
	}

	mValidConfig = utils2.MakeGauge("validation.gateway.solo.io/valid_config",
		"A boolean that indicates whether the Gloo configuration is valid. However, its behavior changes depending upon the validation configuration. Configuration status metrics provide a better solution: https://docs.solo.io/gloo-edge/latest/guides/traffic_management/configuration_validation/")
)

const (
	InvalidSnapshotErrMessage = "validation is disabled due to an invalid resource which has been written to storage. " +
		"Please correct any Rejected resources to re-enable validation."
)

var _ Validator = &validator{}

type Validator interface {
	gloov1snap.ApiSyncer
	// ValidateList will validate a list of resources
	ValidateList(ctx context.Context, ul *unstructured.UnstructuredList, dryRun bool) (*Reports, *multierror.Error)
	// ValidateModifiedGvk validate the creation or update of a resource.
	ValidateModifiedGvk(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) (*Reports, error)
	// ValidateDeletedGvk validate the deletion of a resource.
	ValidateDeletedGvk(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) error
}

type GlooValidatorFunc = func(ctx context.Context, proxy *gloov1.Proxy,
	resource resources.Resource, shouldDelete bool,
) ([]*gloovalidation.GlooValidationReport, error)

type validator struct {
	lock              sync.RWMutex
	latestSnapshot    *gloov1snap.ApiSnapshot
	latestSnapshotErr error
	translator        translator.Translator
	// This function replaces a grpc client from when gloo and gateway pods were separate.
	glooValidator      GlooValidatorFunc
	extensionValidator syncerValidation.Validator
	allowWarnings      bool
}

type validationOptions struct {
	Ctx         context.Context
	AcquireLock bool
	DryRun      bool
	Delete      bool
	SkipDelete  bool
	Resource    resources.Resource
	Gvk         schema.GroupVersionKind
}

type ValidatorConfig struct {
	Translator         translator.Translator
	GlooValidator      GlooValidatorFunc
	ExtensionValidator syncerValidation.Validator
	AllowWarnings      bool
}

func NewValidator(cfg ValidatorConfig) *validator {
	return &validator{
		glooValidator:      cfg.GlooValidator,
		extensionValidator: cfg.ExtensionValidator,
		translator:         cfg.Translator,
		allowWarnings:      cfg.AllowWarnings,
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
			errs = multierror.Append(errs, err)
		}
	}

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

func (v *validator) gatewayUpdate(snap *gloov1snap.ApiSnapshot) bool {

	if v.latestSnapshot == nil {
		return true
	}
	//look at the hash of resources that affect the gateway snapshot
	hashFunc := func(snap *gloov1snap.ApiSnapshot) (uint64, error) {
		toHash := append([]interface{}{}, snap.VirtualHostOptions.AsInterfaces()...)
		toHash = append(toHash, snap.VirtualServices.AsInterfaces()...)
		toHash = append(toHash, snap.Gateways.AsInterfaces()...)
		toHash = append(toHash, snap.RouteOptions.AsInterfaces()...)
		toHash = append(toHash, snap.RouteTables.AsInterfaces()...)
		toHash = append(toHash, snap.HttpGateways.AsInterfaces()...)
		toHash = append(toHash, snap.VirtualHostOptions.AsInterfaces()...)
		hash, err := hashutils.HashAllSafe(nil, toHash...)
		if err != nil {
			contextutils.LoggerFrom(context.Background()).DPanic("this error should never happen, as this is safe hasher")
			return 0, errors.New("this error should never happen, as this is safe hasher")
		}
		return hash, nil
	}
	oldHash, oldHashErr := hashFunc(v.latestSnapshot)
	newHash, newHashErr := hashFunc(snap)

	// If we cannot hash then we choose to treat them as different hashes since this is just a performance optimization.
	// In worst case we'd prefer correctness
	hashChanged := oldHash != newHash || oldHashErr != nil || newHashErr != nil
	return hashChanged
}

func (v *validator) validateSnapshotThreadSafe(opts *validationOptions) (
	*Reports,
	error,
) {
	v.lock.Lock()
	defer v.lock.Unlock()

	return v.validateSnapshot(opts)
}

// type proxyTranslationResponse struct {
// 	Proxies      []*gloov1.Proxy
// 	ProxyReports ProxyReports
// 	Error        error
// 	ExtraError   error
// }

func (v *validator) translateProxies(ctx context.Context, snapshot *gloov1snap.ApiSnapshot, opts *validationOptions) ([]*gloov1.Proxy, ProxyReports, error) {
	var (
		errs         error
		err          error
		proxyReports ProxyReports
		proxies      []*gloov1.Proxy
	)

	getAllErrors := opts.Delete && opts.Gvk.Kind == "Secret" // We're overriding `delete` so need another way to check
	gatewaysByProxy := utils.GatewaysByProxyName(snapshot.Gateways)
	// translate all the proxies
	for proxyName, gatewayList := range gatewaysByProxy {
		proxy, reports := v.translator.Translate(ctx, proxyName, snapshot, gatewayList)
		validate := reports.ValidateStrict
		if v.allowWarnings {
			validate = reports.Validate
		}
		if err := validate(); err != nil {
			fmt.Printf("SAH - error translating proxy: %v\n", err)
			err = errors.Wrapf(err, couldNotRenderProxy)
			errs = multierror.Append(errs, err)

			if !getAllErrors {
				continue
			}
		}

		//printUnwrappedMultiError(errs, "SAH - After translating proxy")

		// a nil proxy may have been returned if 0 listeners were created
		if proxy == nil {
			continue
		}

		proxies = append(proxies, proxy)

		//fmt.Println("SAH - v.glooValidator")
		// validate the proxy with gloo this occurs in projects/gloo/pkg/validation/validator.go
		glooReports, err := v.glooValidator(ctx, proxy, opts.Resource, (opts.Delete && !opts.SkipDelete))
		if err != nil {
			//fmt.Printf("SAH - glooValidator error : %v\n", err)
			err = errors.Wrapf(err, failedGlooValidation)
			//err = multiErrorWrap(err, failedGlooValidation)
			errs = multierror.Append(errs, err)
			if !getAllErrors {
				continue
			}

			//printUnwrappedMultiError(errs, "SAH - After glooValidator")
		}

		if len(glooReports) != 1 {
			// This was likely caused by a development error
			err := GlooValidationResponseLengthError(glooReports)
			//fmt.Printf("SAH - GlooValidationResponseLengthError error : %v\n", err)
			errs = multierror.Append(errs, err)
			//continue
		}

		proxyReport := glooReports[0].ProxyReport
		proxyReports = append(proxyReports, proxyReport)
		if err := validationutils.GetProxyError(proxyReport); err != nil {
			errs = multierror.Append(errs, proxyFailedGlooValidation(err, proxy))
			//fmt.Printf("SAH - proxyReports error : %v\n", err)
			if !getAllErrors {
				continue
			}
		}
		if warnings := validationutils.GetProxyWarning(proxyReport); !v.allowWarnings && len(warnings) > 0 {
			for _, warning := range warnings {
				//fmt.Printf("SAH - GetProxyWarning : %v\n", err)
				errs = multierror.Append(errs, errors.New(warning))
			}
			if !getAllErrors {
				continue
			}
		}

		//printUnwrappedMultiError(errs, "SAH - After GetProxyWarning")

		err = v.getErrorsFromGlooValidation(glooReports)
		if err != nil {
			//fmt.Printf("SAH - getErrorsFromGlooValidation : %v\n", err)
			//err = errors.Wrapf(err, failedResourceReports)
			//printUnwrappedMultiError(err, "SAH - Returned from getErrorsFromGlooValidation")
			err = multiErrorWrap(err, failedResourceReports)
			errs = multierror.Append(errs, err)
			if !getAllErrors {
				continue
			}
		}

		//printUnwrappedMultiError(errs, "SAH - After getErrorsFromGlooValidation")
	}

	extensionReports := v.extensionValidator.Validate(ctx, snapshot)
	if len(extensionReports) > 0 {
		if err = v.getErrorsFromResourceReports(extensionReports); err != nil {
			fmt.Printf("SAH - extensionReports errors : %v\n", err)
			err = errors.Wrapf(err, failedExtensionResourceReports)
			//err = multiErrorWrap(err, failedExtensionResourceReports)
			errs = multierror.Append(errs, err)
		}
	}

	//printUnwrappedMultiError(errs, "SAH - Before return from translateProxies")

	return proxies, proxyReports, errs
}

func multiErrorWrap(err error, s string) error {
	if err == nil {
		return nil
	}

	var retErrs error
	if merr, ok := err.(*multierror.Error); ok {
		for _, err := range merr.WrappedErrors() {
			errors.Wrapf(err, s)
			fmt.Println("SAH - wrapped up error: ", err)
			retErrs = multierror.Append(retErrs, err)
		}
	} else {
		retErrs = errors.Wrapf(err, s)
	}

	return retErrs

}

// func printUnwrappedMultiError(err error, s string) {

// 	if merr, ok := err.(*multierror.Error); ok {
// 		unwrapped := merr.WrappedErrors()

// 		fmt.Println("SAH - Unwrapped errors: ", s)
// 		for _, err := range unwrapped {
// 			fmt.Println("+++\n", err, "\n+++")
// 		}
// 	} else {
// 		fmt.Println(s, "not a multierror")
// 	}

// }

func (v *validator) validateSnapshot(opts *validationOptions) (*Reports, error) {
	fmt.Println("SAH - validateSnapshot")
	// validate that a snapshot can be modified
	// should be called within a lock
	//
	// validation occurs by the following steps:
	//	1. Clone the most recent snapshot
	//	2. Apply the changes to that snapshot clone
	//	3. Validate the generated proxy of that snapshot clone by validating both gateway and gloo translation.
	//		a. we call gloo translation via a passed method, glooValidator
	//	4. If the proxy is valid, we know that the requested mutation is valid. If this request happens
	//		during a dry run, we don't want to actually apply the change, since this will modify the internal
	//		state of the validator, which is shared across requests. Therefore, only if we are not in a dry run,
	//		we apply the mutation.
	ctx := opts.Ctx
	if !v.ready() {
		return nil, NotReadyErr
	}
	ref := opts.Resource.GetMetadata().Ref()
	ctx = contextutils.WithLogger(ctx, "gateway-validator")

	// currently have the other for Gloo resources
	snapshotClone, err := v.copySnapshotNonThreadSafe(ctx, opts.DryRun)
	if err != nil {
		// allow writes if storage is already broken
		return nil, nil
	}

	//var secretToSave gloov1.Secret
	// verify the mutation against a snapshot clone first, only apply the change to the actual snapshot if this passes
	if opts.Delete {
		fmt.Println("SAH - removing from resource list")
		if err := snapshotClone.RemoveFromResourceList(opts.Resource); err != nil {
			return nil, err
		}
	} else {
		fmt.Println("SAH - upserting resource list")
		if err := snapshotClone.UpsertToResourceList(opts.Resource); err != nil {
			return nil, err
		}
	}

	proxies, proxyReports, errs := v.translateProxies(ctx, snapshotClone, opts)

	//printUnwrappedMultiError(errs, "SAH - All validation errors")

	// if merr, ok := errs.(*multierror.Error); ok {
	// 	errList := merr.WrappedErrors()

	// 	fmt.Println("SAH - Unwrapped errors: ")
	// 	for _, e := range errList {
	// 		if merr2, ok := errs.(*multierror.Error); ok {
	// 			errList2 := merr2.WrappedErrors()

	// 			for _, e2 := range errList2 {
	// 				fmt.Printf("+++2 deep\n%+v\n+++\n", e2)
	// 			}
	// 		} else {
	// 			fmt.Printf("+++no unwrapping\n%+v\n+++\n", e)
	// 		}
	// 	}
	// } else {
	// 	fmt.Println("SAH - unable to cast to multierror")
	// }

	//fmt.Println("SAH - proxyReports: ", proxyReports)
	//fmt.Println("SAH -- errors: ", errs)
	// // If we're deleting a secret and run into an error, try to revaldiate
	okToDeleteSecret := false
	if errs != nil && opts.Delete && opts.Gvk.Kind == "Secret" {
		fmt.Println("SAH - revalidating secret delete")

		opts.SkipDelete = true
		// fmt.Println("Adding back to resource list")
		// if err := snapshotClone.UpsertToResourceList(opts.Resource); err != nil {
		// 	return nil, err
		// }

		snapshotCloneUnmodified, err := v.copySnapshotNonThreadSafe(ctx, opts.DryRun)
		if err != nil {
			// allow writes if storage is already broken
			return nil, nil
		}

		origProxies, origProxyReports, origErrs := v.translateProxies(ctx, snapshotCloneUnmodified, opts)

		fmt.Println("SAH - compare errors")

		// Check errors
		var errList []error
		var errListOrig []error

		if merr, ok := errs.(*multierror.Error); ok {
			errList = merr.WrappedErrors()
		} else {
			errList = []error{errs}
		}

		if merr, ok := origErrs.(*multierror.Error); ok {
			errListOrig = merr.WrappedErrors()
		} else {
			errListOrig = []error{origErrs}
		}

		sameErrors := len(errList) == len(errListOrig)

		if sameErrors {
			origErrMap := make(map[string]bool)
			for _, e := range errList {
				fmt.Printf("SAH - origErr: +++\n%s\n+++ \n", e.Error())
				origErrMap[e.Error()] = true
			}

			for _, e := range errListOrig {
				if _, ok := origErrMap[e.Error()]; !ok {
					fmt.Printf("SAH - error not in origErrMap:\n+++\n%+v\n++++\n", e)
					sameErrors = false
					break
				}
			}
		}

		//fmt.Println("SAH - origProxyReports-2: ", origProxyReports)
		//fmt.Println("SAH -- origErrors-2: ", origErrs)
		// Compare proxies - move to a function
		sameProxies := len(origProxies) == len(proxies)
		if sameProxies {
			proxyHashes := make(map[string]uint64)
			///origProxyhashes = make(map[string]uint64)
			for _, proxy := range proxies {
				// Need better key, put in function
				proxyKey := fmt.Sprintf("%s-%s", proxy.GetMetadata().GetNamespace(), proxy.GetMetadata().GetName())
				proxyHashes[proxyKey] = proxy.MustHash()
			}

			for _, proxy := range origProxies {
				proxyKey := fmt.Sprintf("%s-%s", proxy.GetMetadata().GetNamespace(), proxy.GetMetadata().GetName())
				origHash := proxy.MustHash()
				fmt.Printf("SAH - proxy %s hash with delete: %d, original hash %d\n", proxyKey, proxyHashes[proxyKey], origHash)
				if proxyHashes[proxyKey] != origHash {
					sameProxies = false
					break
				}
			}
		}

		fmt.Printf("SAH Proxies are the same? %t\n", sameProxies)

		sameReports := len(origProxyReports) == len(proxyReports)
		if sameReports {
			proxyReportMap := make(map[string]bool)
			for _, proxyReport := range proxyReports {
				proxyReportMap[proxyReport.String()] = true
			}

			// Check each report against the map
			for _, proxyReport := range origProxyReports {
				if _, ok := proxyReportMap[proxyReport.String()]; !ok {
					sameReports = false
					//break
				}
			}
		}

		fmt.Printf("SAH Reports are the same? %t\n", sameReports)

		if !sameReports {
			fmt.Printf("SAH - ProxyReports with delete: %v\n", proxyReports)
			fmt.Printf("SAH - origProxyReports: %v\n", origProxyReports)
		}
		// fmt.Printf("SAH - origProxyReports: %v\n", origProxyReports)
		// fmt.Printf("SAH - ProxyReports with delete: %v\n", proxyReports)

		// fmt.Printf("SAH - origErrors: %v\n", origErrs)
		// fmt.Printf("SAH - errors: %v\n", errs)

		if sameProxies && sameErrors && sameReports {
			okToDeleteSecret = true
		}
	}

	if errs != nil {
		if !opts.DryRun {
			utils2.MeasureZero(ctx, mValidConfig)
		}

		if okToDeleteSecret {
			contextutils.LoggerFrom(ctx).Debugf("Found Errors, but deleting secret: %T %v: %v", opts.Resource, ref, errs)
		} else {
			contextutils.LoggerFrom(ctx).Debugf("Rejected %T %v: %v", opts.Resource, ref, errs)

			return &Reports{ProxyReports: &proxyReports, Proxies: proxies}, errors.Wrapf(errs,
				"validating %T %v",
				opts.Resource,
				ref)
		}
	}

	contextutils.LoggerFrom(ctx).Debugf("Accepted %T %v", opts.Resource, ref)
	if !opts.DryRun && !okToDeleteSecret {
		utils2.MeasureOne(ctx, mValidConfig)
	}

	reports := &Reports{ProxyReports: &proxyReports, Proxies: proxies}
	if !opts.DryRun {
		// update internal snapshot to handle race where a lot of resources may be applied at once, before syncer updates
		if opts.Delete {
			fmt.Println("SAH - removing from resource list for real delete")
			if err = v.latestSnapshot.RemoveFromResourceList(opts.Resource); err != nil {
				return reports, err
			}
		} else {
			if err = v.latestSnapshot.UpsertToResourceList(opts.Resource); err != nil {
				return reports, err
			}
		}
	}

	return reports, nil
}

// ValidateDeletedGvk will validate a deletion of a resource, as long as it is supported, against the Gateway and Gloo Translations.
func (v *validator) ValidateDeletedGvk(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) error {
	fmt.Println("SAH - ValidateDeletedGvk")
	_, err := v.validateResource(&validationOptions{Ctx: ctx, Resource: resource, Gvk: gvk, Delete: true, DryRun: dryRun, AcquireLock: true})
	return err
}

// ValidateModifiedGvk will validate a resource, as long as it is supported, against the Gateway and Gloo translations.
// The resource should be updated or created.  Use Validate Delete Gvk for deleted resources.
func (v *validator) ValidateModifiedGvk(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) (*Reports, error) {
	return v.validateModifiedResource(ctx, gvk, resource, dryRun, true)
}

func (v *validator) validateModifiedResource(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun, acquireLock bool) (*Reports, error) {
	var reports *Reports
	reports, err := v.validateResource(&validationOptions{Ctx: ctx, Resource: resource, Gvk: gvk, Delete: false, DryRun: dryRun, AcquireLock: acquireLock})
	if err != nil {
		return reports, &multierror.Error{Errors: []error{errors.Wrapf(err, "Validating %T failed", resource)}}
	}
	return reports, nil
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

		// this will lock
		var itemProxyReports, err = v.processItem(ctx, item)

		errs = multierror.Append(errs, err)
		if itemProxyReports != nil && itemProxyReports.ProxyReports != nil {
			// ok to return final proxy reports as the latest result includes latest proxy calculated
			// for each resource, as we process incrementally, storing new state in memory as we go
			proxyReports = append(proxyReports, *itemProxyReports.ProxyReports...)
			proxies = append(proxies, itemProxyReports.Proxies...)
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

	if newResourceFunc, hit := gloosnapshot.ApiGvkToHashableResource[itemGvk]; hit {
		resource := newResourceFunc()
		if unmarshalErr := UnmarshalResource(jsonBytes, resource); unmarshalErr != nil {
			return &Reports{ProxyReports: &ProxyReports{}}, WrappedUnmarshalErr(unmarshalErr)
		}
		return v.validateModifiedResource(ctx, itemGvk, resource, false, false)
	}
	// should not happen
	return &Reports{ProxyReports: &ProxyReports{}}, errors.Errorf("Unknown group/version/kind, %v", itemGvk)
}

// copySnapshotNonThreadSafe will copy the snapshot. If there is an error with the latest snapshot, it will error.
// NOTE: does not perform any lock, and this function is not thread safe. Any read or write to the snapshot needs to be
// done under a lock
func (v *validator) copySnapshotNonThreadSafe(ctx context.Context, dryRun bool) (*gloosnapshot.ApiSnapshot, error) {
	if v.latestSnapshot == nil {
		return nil, HasNotReceivedFirstSync
	}
	if v.latestSnapshotErr != nil {
		if !dryRun {
			utils2.MeasureZero(ctx, mValidConfig)
		}
		contextutils.LoggerFrom(ctx).Errorw(InvalidSnapshotErrMessage, zap.Error(v.latestSnapshotErr))
		return nil, eris.New(InvalidSnapshotErrMessage)
	}
	snapshotClone := v.latestSnapshot.Clone()
	return &snapshotClone, nil
}

func (v *validator) validateResource(opts *validationOptions) (*Reports, error) {
	if opts.AcquireLock {
		return v.validateSnapshotThreadSafe(opts)
	} else {
		return v.validateSnapshot(opts)
	}
}

// getErrorsFromGlooValidation returns an error comprising of the gloo reports. The errors will include warnings if
// allowWarnings is not set.
func (v *validator) getErrorsFromGlooValidation(reports []*gloovalidation.GlooValidationReport) error {
	var errs error

	for _, report := range reports {
		if err := v.getErrorsFromResourceReports(report.ResourceReports); err != nil {
			errs = multierror.Append(errs, err)
		}
		if proxyReport := report.ProxyReport; proxyReport != nil {
			if err := validationutils.GetProxyError(proxyReport); err != nil {
				errs = multierror.Append(errs, errors.Wrapf(err, "failed to validate Proxy with Gloo validation server"))
			}
			if warnings := validationutils.GetProxyWarning(proxyReport); !v.allowWarnings && len(warnings) > 0 {
				for _, warning := range warnings {
					errs = multierror.Append(errs, errors.New(warning))
				}
			}
		}
	}

	return errs
}

func (v *validator) getErrorsFromResourceReports(reports reporter.ResourceReports) error {
	if !v.allowWarnings {
		return reports.ValidateStrict()
	}
	return reports.Validate()
}

// UnmarshalResource is the same as the solo-kit pkg/utils/protoutils.Unmarshal() except it does not set the status of the resource
// since validation does not write the resources, this is ok. Validation will only store the state of a resource
// to the copy of the snapshot.
func UnmarshalResource(kubeJson []byte, resource resources.Resource) error {
	var resourceCrd kubeCRDV1.Resource
	if err := json.Unmarshal(kubeJson, &resourceCrd); err != nil {
		return errors.Wrapf(err, "unmarshalling from raw json")
	}
	resource.SetMetadata(kubeutils.FromKubeMeta(resourceCrd.ObjectMeta, true))

	if resourceCrd.Spec != nil {
		if cir, ok := resource.(resources.CustomInputResource); ok {
			// Custom input resource unmarshalling
			if err := cir.UnmarshalSpec(*resourceCrd.Spec); err != nil {
				return errors.Wrapf(err, "parsing custom input resource from crd spec %v in namespace %v into %T", resourceCrd.Name, resourceCrd.Namespace, resource)
			}
		} else if err := skProtoUtils.UnmarshalMap(*resourceCrd.Spec, resource); err != nil {
			// Default unmarshalling
			return errors.Wrapf(err, "parsing resource from crd spec %v in namespace %v into %T", resourceCrd.Name, resourceCrd.Namespace, resource)
		}
	}
	return nil
}
