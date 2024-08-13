package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	utils2 "github.com/solo-io/gloo/pkg/utils/statsutils"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/go-utils/hashutils"

	"github.com/hashicorp/go-multierror"
	errors "github.com/rotisserie/eris"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	k8sgwvalidation "github.com/solo-io/gloo/projects/gateway2/validation"
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

// This error is a struct so it can be checked with errors.As
type GlooValidationResponseLengthError struct {
	reportLength int
}

func (e GlooValidationResponseLengthError) Error() string {
	return fmt.Sprintf("Expected Gloo validation response to contain 1 report, but contained %d", e.reportLength)
}

// This error is a struct so it can be checked with errors.As
type SyncNotYetRunError struct {
	err error
}

func (e SyncNotYetRunError) Error() string {
	return errors.Wrap(e.err, failedGlooValidation).Error()
}

var (
	NotReadyErr                    = errors.Errorf("validation is not yet available. Waiting for first snapshot")
	HasNotReceivedFirstSync        = errors.New("proxy validation called before the validation server received its first sync of resources")
	unmarshalErrMsg                = "could not unmarshal raw object"
	couldNotRenderProxy            = "could not render proxy"
	failedGlooValidation           = "failed gloo validation"
	failedResourceReports          = "failed gloo validation resource reports"
	failedExtensionResourceReports = "failed extension resource reports"
	WrappedUnmarshalErr            = func(err error) error {
		return errors.Wrapf(err, unmarshalErrMsg)
	}

	proxyFailedGlooValidation = func(err error, proxy *gloov1.Proxy) error {
		return errors.Wrapf(err, "failed to validate Proxy [namespace: %s, name: %s] with gloo validation", proxy.GetMetadata().GetNamespace(), proxy.GetMetadata().GetName())
	}

	mValidConfig = utils2.MakeGauge("validation.gateway.solo.io/valid_config",
		"A boolean that indicates whether the Gloo configuration is valid. However, its behavior changes depending upon the validation configuration. Configuration status metrics provide a better solution: https://docs.solo.io/gloo-edge/latest/guides/traffic_management/configuration_validation/")

	BreakingErrorLogMsg = "Breaking errors found, not revalidating against original snapshot"
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
	glooValidator                         GlooValidatorFunc
	extensionValidator                    syncerValidation.Validator
	allowWarnings                         bool
	disableValidationAgainstPreviousState bool
}

type validationOptions struct {
	Ctx         context.Context
	AcquireLock bool
	DryRun      bool
	Delete      bool
	Resource    resources.Resource
	Gvk         schema.GroupVersionKind
	// This flag is used when re-validating a snapshot when deleting secrets and is used in setting
	//  the `resource` parameter passed to the glooValidator, which will remove the resource if it is present
	validateUnmodified bool
	// When we may be comparing the output of validation with the original validation output, we want to collect all errors instead of returning on the first error
	collectAllErrors bool
}

type ValidatorConfig struct {
	Translator                            translator.Translator
	GlooValidator                         GlooValidatorFunc
	ExtensionValidator                    syncerValidation.Validator
	AllowWarnings                         bool
	DisableValidationAgainstPreviousState bool
}

func NewValidator(cfg ValidatorConfig) *validator {
	return &validator{
		glooValidator:                         cfg.GlooValidator,
		extensionValidator:                    cfg.ExtensionValidator,
		translator:                            cfg.Translator,
		allowWarnings:                         cfg.AllowWarnings,
		disableValidationAgainstPreviousState: cfg.DisableValidationAgainstPreviousState,
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

type validationOutput struct {
	proxies      []*gloov1.Proxy
	proxyReports ProxyReports
	err          error
}

// validateProxiesAndExtensions validates a snapshot against the Gloo and Gateway Translations. The supplied snapshot should have either been
// modified to contain the resource being validated or in the case of DELETE validation, have the resource in question removed.
// This was removed from the main validation loop to allow it to be re-run against the original snapshot.
// The reason for this second validaiton run is to allow the deletion of secrets, but only if they are not in use by the snapshot.
// This function does not know about those use cases, but supports them with the opts.collectAllErrors flag, which is passed as 'true' when
// attempting to delete a secret. This flag overrides the usual behavior of continuing to the next proxy after the first error,
// and instead collects all errors.
//
// This means there are four separate behaviors for validation:
// 1. allow_warnings=true and opts.collectAllErrors=false
//     Warnings are ignored and after the first error for a proxy validation stops and the next proxy is translated/validated
// 2. allow_warnings=true and opts.collectAllErrors=true
//     Warnings are ignored, and all errors are collected and returned
// 3. allow_warnings=false and opts.collectAllErrors=false
//     Warnings are treated as errors and after the first error for a proxy validation stops and the next proxy is translated
// 4. allow_warnings=false and opts.collectAllErrors=true
//     Warnings are treated as errors and all errors are collected and returned
//
// There are two main ways errors and warnings are collected to be processed:
// 1. The Gloo validation reports are collected and processed by the reporter package. Based on the value of `allowWarnings`,
//     errors are extracted from the reports by `ValidateStrict` which returns warnings as errors or `Validate`, which ignores warnings.
// 2. Manually looping over proxyreport errors and warnings. In these cases, the `v.allowWarnings` flag is used to determine whether
//     to include the warnings with the errors.
//
// The output of this function is:
// []*gloov1.Proxy - proxies that were generated from the snapshot
// ProxyReports - the reports from the Gloo validation
// error - errors from the Gloo validation

// When validating reports with the reporter package errors and warnings are sorted how we want them to be returned
// other sources of warnings/errors need to be handled separately
func (v *validator) validateProxiesAndExtensions(ctx context.Context, snapshot *gloov1snap.ApiSnapshot, opts *validationOptions) validationOutput {
	var (
		proxies      []*gloov1.Proxy
		proxyReports ProxyReports
		errs         error
		err          error
	)

	validatingK8sGateway := isK8sGatewayProxy(opts.Resource)
	if validatingK8sGateway {
		proxies, errs = k8sgwvalidation.TranslateK8sGatewayProxies(ctx, snapshot, opts.Resource)
	} else {
		proxies, errs = v.translateGlooEdgeProxies(ctx, snapshot, opts.collectAllErrors)
	}

	// Now perform gloo validation for all the Proxies
	for _, proxy := range proxies {
		// Validate the proxy with the Gloo validator
		// This validation also attempts to modify the snapshot, so when validating the unmodified snapshot a nil resource is passed in so no modifications are made
		resourceToModify := opts.Resource
		if opts.validateUnmodified {
			resourceToModify = nil
		}

		// The error returned here will occur when the function is run before the first sync of resources
		// If we encounter this error we can `continue` even if collecting all errors, as we know
		// the revalidation will fail due to the presence of this error
		glooReports, err := v.glooValidator(ctx, proxy, resourceToModify, opts.Delete)
		if err != nil {
			err = SyncNotYetRunError{err: err}
			errs = multierror.Append(errs, err)
			continue
		}

		if len(glooReports) != 1 {
			// This was likely caused by a development error. When passing a proxy to the glooValidator,  it should return a single report:
			// https://github.com/solo-io/gloo/blob/85a8f3f509f47d93e877b932e9785998215210c5/projects/gloo/pkg/validation/validator.go#L55
			// If this error is encountered, stop collecting all errors, as revalidation will fail due to the presence of this error
			err = GlooValidationResponseLengthError{reportLength: len(glooReports)}
			errs = multierror.Append(errs, err)
			continue
		}

		// if we are validating K8s Gateway Policy resources, we only want the errors resulting from
		// the RouteOption/VirtualHostOption, so let's grab that here and skip the more sophisticated
		// error aggregation below. Note that we do not care about allowWarnings, as they are not
		// used in this case, as we do not do a full translation and use a 'dummy' proxy
		if validatingK8sGateway {
			if err = k8sgwvalidation.GetSimpleErrorFromGlooValidation(glooReports, proxy); err != nil {
				errs = multierror.Append(errs, err)
			}
			continue
		}

		// Collect the reports returned by the glooValidator
		proxyReport := glooReports[0].ProxyReport
		proxyReports = append(proxyReports, proxyReport)

		// Validate the reports returned by the glooValidator
		// Get the errors and warnings from the proxyReport
		stopValidatingProxy := v.appendProxyErrors(&errs, proxyReport, proxy, opts)
		if stopValidatingProxy {
			continue
		}

		// Get errors from the glooReports
		// The returned value indicates whether to stop processing this proxy, but this is the end of the loop
		err = v.getErrorsFromGlooValidation(glooReports)
		if err != nil {
			err = errors.Wrapf(err, failedResourceReports)
			errs = multierror.Append(errs, err)
		}
	} // End of proxy validation loop

	// Extension validation. Currently only supports rate limit.
	extensionReports := v.extensionValidator.Validate(ctx, snapshot)

	if len(extensionReports) > 0 {
		// Collect the errors from the reports
		err = v.getErrorsFromResourceReports(extensionReports)

		if err != nil {
			err = errors.Wrapf(err, failedExtensionResourceReports)
			errs = multierror.Append(errs, err)
		}
	}

	return validationOutput{
		proxies:      proxies,
		proxyReports: proxyReports,
		err:          errs,
	}
}

// the returned error is a multierror that may contain errors from several Edge Proxies, although this depends on collectAllErrors
func (v *validator) translateGlooEdgeProxies(
	ctx context.Context,
	snapshot *gloov1snap.ApiSnapshot,
	collectAllErrors bool,
) ([]*gloov1.Proxy, error) {
	var (
		proxies []*gloov1.Proxy
		errs    error
	)

	gatewaysByProxy := utils.SortedGatewaysByProxyName(snapshot.Gateways)

	// translate all the proxies
	for _, gatewayAndProxyName := range gatewaysByProxy {
		proxyName := gatewayAndProxyName.Name
		gatewayList := gatewayAndProxyName.Gateways

		// Translate the proxy and process the errors
		proxy, reports := v.translator.Translate(ctx, proxyName, snapshot, gatewayList)

		err := v.getErrorsFromResourceReports(reports)

		if err != nil {
			err = errors.Wrapf(err, couldNotRenderProxy)
			errs = multierror.Append(errs, err)

			if !collectAllErrors {
				continue
			}
		}

		// A nil proxy may have been returned if 0 listeners were created
		// continue here even if collecting all errors, because the proxy is nil and there is nothing to validate
		if proxy == nil {
			continue
		}
		proxies = append(proxies, proxy)
	}

	return proxies, errs
}

// isK8sGatewayProxy returns true if we are evaluating a Policy resource in the context of K8s Gateway API support.
// Currently the only signal needed to make this decision is if the resource being evaluated is a RouteOption
// or a VirthalHostOption, as we only directly evaluate them in the validator to support K8s Gateway API mode.
func isK8sGatewayProxy(res resources.Resource) bool {
	switch res.(type) {
	case *sologatewayv1.RouteOption, *sologatewayv1.VirtualHostOption:
		return true
	default:
		return false
	}
}

// appendProxyErrors appends the errors and from the proxyReport to the (multi)error, passed in by pointer
// It returns a boolean to indicate whether the caller should continue processing the next proxy
func (v *validator) appendProxyErrors(errs *error, proxyReport *validation.ProxyReport, proxy *gloov1.Proxy, opts *validationOptions) bool {
	if err := validationutils.GetProxyError(proxyReport); err != nil {
		*errs = multierror.Append(*errs, proxyFailedGlooValidation(err, proxy))

		if !opts.collectAllErrors {
			return true
		}
	}

	// Get the warnings from the proxyReport
	if proxyWarnings := validationutils.GetProxyWarning(proxyReport); !v.allowWarnings && len(proxyWarnings) > 0 {
		for _, warning := range proxyWarnings {
			*errs = multierror.Append(*errs, errors.New(warning))
		}
		if !opts.collectAllErrors {
			return true
		}
	}

	return false
}

func (v *validator) validateSnapshot(opts *validationOptions) (*Reports, error) {
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
	//
	//	There is a variation on this process if the requested mutation is a deletion of a secret.
	//	In this case deletion of the secret is allowed if not in use by the snapshot.
	//	This is done by running validation on (a clone of) the original snapshot without the secret removed.
	//	If the output is the same, as the run with the secret removed, then the secret is not in use and can be deleted.
	//	Otherwise, the secret is in use and cannot be deleted.
	//	This logic is gated in <subroutine> and other types of resources may be added in the future.

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

	// verify the mutation against a snapshot clone first, only apply the change to the actual snapshot if this passes
	if opts.Delete {
		if err := snapshotClone.RemoveFromResourceList(opts.Resource); err != nil {
			return nil, err
		}
	} else {
		if err := snapshotClone.UpsertToResourceList(opts.Resource); err != nil {
			return nil, err
		}
	}

	// In some cases errors do not result in an automatic rejection of the modifcation. In those cases, all errors are collected and returned
	// so they can be compared against the result of a second validation run of the current, unmodified snapshot
	shouldValidateAgainstPreviousState := v.shouldValidateAgainstPreviousState(opts)

	// The collectAllErrors opts field is used to control whether all errors are collected or if valdiation for a proxy is stopped on the first error
	opts.collectAllErrors = shouldValidateAgainstPreviousState

	// Run the validation.
	validationOutput := v.validateProxiesAndExtensions(ctx, snapshotClone, opts)
	//errs := validationOutput.err // we use this one so often ,just pull it out

	passedSnapshotValidation := false
	// We want to compare the validation output if the retryValidation flag and we are currently not passing validation
	if shouldValidateAgainstPreviousState && validationOutput.err != nil {
		passedSnapshotValidation = v.validateAgainstCurrentSnapshot(ctx, opts, validationOutput)
	}

	// Put the metric logic in its own block because the acceptance logic has gotten more complicated
	if !opts.DryRun {
		if validationOutput.err == nil {
			utils2.MeasureOne(ctx, mValidConfig)
		} else {
			utils2.MeasureZero(ctx, mValidConfig)
		}
	}

	if validationOutput.err != nil && !passedSnapshotValidation {
		contextutils.LoggerFrom(ctx).Debugf("Rejected %T %v: %v", opts.Resource, ref, validationOutput.err)
		return &Reports{ProxyReports: &validationOutput.proxyReports, Proxies: validationOutput.proxies}, errors.Wrapf(validationOutput.err,
			"validating %T %v",
			opts.Resource,
			ref)

	}

	contextutils.LoggerFrom(ctx).Debugf("Accepted %T %v", opts.Resource, ref)

	reports := &Reports{ProxyReports: &validationOutput.proxyReports, Proxies: validationOutput.proxies}
	if !opts.DryRun {
		// update internal snapshot to handle race where a lot of resources may be applied at once, before syncer updates
		if opts.Delete {
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

// shouldValidateAgainstPreviousState contains the logic to determine if validation should be retried against the original snapshot
// and the results of that validation compared to the original validation output in order to determine whether to accept the modification.
// Currently we only support this for the deletion of secrets.
func (v *validator) shouldValidateAgainstPreviousState(opts *validationOptions) bool {
	if v.disableValidationAgainstPreviousState {
		return false
	}

	// If the resource is a secret, and the delete flag is set, and the 'allow_warnings' flag is set to false.
	if opts.Delete && opts.Gvk.Kind == "Secret" {
		return true
	}
	return false
}

// validateAgainstCurrentSnapshot is used to compare the output of validation against validation of the original snapshot.
// It takes the output of the first validation with the modification and then gets the output of the validation of the original snapshot
// and compares the two using compareValidationOutputs, unless there are breaking errors in the first validation output
func (v *validator) validateAgainstCurrentSnapshot(ctx context.Context, opts *validationOptions, voMod validationOutput) bool {
	contextutils.LoggerFrom(ctx).Debugw(
		"Comparing validation output against original snapshot",
		zap.String("resource", opts.Resource.GetMetadata().String()),
	)

	if findBreakingErrors(voMod.err) {
		contextutils.LoggerFrom(ctx).Debug(BreakingErrorLogMsg)
		return false
	}
	// Set the 'validateUnmodified' flag to true to ensure that the resource is not deleted in glooValidation
	opts.validateUnmodified = true

	snapshotCloneUnmodified, err := v.copySnapshotNonThreadSafe(ctx, opts.DryRun)
	if err != nil {
		// If storage is broken default to to disallowing the update.
		// Don't override the initial errors without being confident that the update is valid
		return false
	}

	// Get the validation output without the modification.
	voNoMod := v.validateProxiesAndExtensions(ctx, snapshotCloneUnmodified, opts)

	return v.compareValidationOutputs(ctx, opts, voMod, voNoMod)
}

// compareValidationOutputs is used to compare the output of validation against validation of the original snapshot
// This is used in special cases, specifically the deletion of a secret.  In these cases, the usual validation logic is overriden,
// and instead of relying on the presence of errors to determine whether to accept the modification, the output of
// validation of the modified snapshot (proxies, proxyReports, errors) is compared to the output of the validation of the original snapshot.
// If outputs are the same, it is assumed that the modification did not degrade the system and  is accepted
func (v *validator) compareValidationOutputs(ctx context.Context, opts *validationOptions, voMod, voNoMod validationOutput) bool {

	sameErrors := compareErrors(voMod.err, voNoMod.err)
	sameProxies := compareProxies(voMod.proxies, voNoMod.proxies)
	sameReports := compareReports(voMod.proxyReports, voNoMod.proxyReports, v.allowWarnings)

	if sameProxies && sameReports && sameErrors {
		contextutils.LoggerFrom(ctx).Debugw(
			"Validation against original snapshot succeded",
			zap.String("resource", opts.Resource.GetMetadata().String()),
		)
		return true
	}

	contextutils.LoggerFrom(ctx).Debugw(
		"Validation against original snapshot failed",
		zap.Bool("sameProxies", sameProxies),
		zap.Bool("sameReports", sameReports),
		zap.Bool("sameErrors", sameErrors),
		zap.String("resource", opts.Resource.GetMetadata().String()),
	)

	return false
}

// compareErrors compares two lists of errors and returns true if they are the same
// The api.snapshot is composed of lists, so the order of validation is consistent.
// Some of the errors are generated by the reporter package, which has been updated to return errors in a consistent order
// Because of this, we can compare errors by comparing the strings of the errors
func compareErrors(err1, err2 error) bool {
	if err1 == nil && err2 == nil {
		return true
	}
	if err1 == nil || err2 == nil {
		return false
	}
	return err1.Error() == err2.Error()
}

// compareReports compares two lists of proxy reports and returns true if they are the same
// The proxy reports can't be compared directly because differences in warnings can be allowed
// This is somewhat redundant as we have already extracted and compared the errors, but provides
// and extra layer of validation.
func compareReports(proxyReports1, proxyReports2 ProxyReports, allowWarnings bool) bool {
	// If warnings are not allowed, we can compare the proxy reports directly
	if !allowWarnings {
		return reflect.DeepEqual(proxyReports1, proxyReports2)
	}

	// Warnings are allowed, so the proxy reports must be compared manually
	if len(proxyReports1) != len(proxyReports2) {
		return false
	}

	// Loop over the proxy reports and compare their errors. Ignore warnings
	for i := range proxyReports1 {
		pr1 := proxyReports1[i]
		pr2 := proxyReports2[i]

		// Check that the listener reports are the same types
		l1 := pr1.GetListenerReports()
		l2 := pr2.GetListenerReports()
		if len(l1) != len(l2) {
			return false
		}
		for i := range l1 {
			// Check that the listener reports are the same types
			if reflect.TypeOf(l1[i].GetListenerTypeReport()) != reflect.TypeOf(l2[i].GetListenerTypeReport()) {
				return false
			}

			// Check that the errors are the same
			e1 := validationutils.GetListenerError(l1[i])
			e2 := validationutils.GetListenerError(l2[i])

			if len(e1) != len(e2) {
				return false
			}

			for j := range e1 {
				if !compareErrors(e1[j], e2[j]) {
					return false
				}
			}
		}
	}

	return true
}

// compareProxies compares two lists of proxies and returns true if they are the same
// First the length of the lists is compared, then the hash of each proxy is compared
// If all of these are the same, the proxies are considered the same
func compareProxies(proxy1, proxy2 []*gloov1.Proxy) bool {
	sameProxies := len(proxy1) == len(proxy2)
	if sameProxies {
		for i := range proxy1 {
			if proxy1[i].MustHash() != proxy2[i].MustHash() {
				return false
			}
		}
	}

	return sameProxies
}

// findBreakingErrors looks for errors that are not due to the snapshot itself,
// for example if Sync has not yet been run. These errors make comparision of snapshot validation output
// invalid for the purposes of determining if an alteration created a new error or warning.
func findBreakingErrors(errs error) bool {
	var lengthError GlooValidationResponseLengthError
	var syncError SyncNotYetRunError

	breakingErrorTypes := []error{
		&lengthError,
		&syncError,
	}

	for _, err := range breakingErrorTypes {
		if errors.As(errs, err) {
			return true
		}
	}

	return false
}

// ValidateDeletedGvk will validate a deletion of a resource, as long as it is supported, against the Gateway and Gloo Translations.
func (v *validator) ValidateDeletedGvk(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) error {
	opts := &validationOptions{
		Ctx:         ctx,
		Resource:    resource,
		Gvk:         gvk,
		Delete:      true,
		DryRun:      dryRun,
		AcquireLock: true,
	}
	_, err := v.validateResource(opts)
	return err
}

// ValidateModifiedGvk will validate a resource, as long as it is supported, against the Gateway and Gloo translations.
// The resource should be updated or created.  Use Validate Delete Gvk for deleted resources.
func (v *validator) ValidateModifiedGvk(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun bool) (*Reports, error) {
	return v.validateModifiedResource(ctx, gvk, resource, dryRun, true)
}

func (v *validator) validateModifiedResource(ctx context.Context, gvk schema.GroupVersionKind, resource resources.Resource, dryRun, acquireLock bool) (*Reports, error) {
	var reports *Reports
	opts := &validationOptions{
		Ctx:         ctx,
		Resource:    resource,
		Gvk:         gvk,
		Delete:      false,
		DryRun:      dryRun,
		AcquireLock: acquireLock,
	}
	reports, err := v.validateResource(opts)
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

	if newResourceFunc, hit := gloov1snap.ApiGvkToHashableResource[itemGvk]; hit {
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
func (v *validator) copySnapshotNonThreadSafe(ctx context.Context, dryRun bool) (*gloov1snap.ApiSnapshot, error) {
	if v.latestSnapshot == nil {
		return nil, HasNotReceivedFirstSync
	}
	if v.latestSnapshotErr != nil {
		if !dryRun {
			utils2.MeasureZero(ctx, mValidConfig)
		}
		contextutils.LoggerFrom(ctx).Errorw(InvalidSnapshotErrMessage, zap.Error(v.latestSnapshotErr))
		return nil, errors.New(InvalidSnapshotErrMessage)
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

// getErrorsFromGlooValidation returns errors from the Gloo validation reports. This function uses the reporter package to
// extract errors and warnings from the reports and manually loops over the proxyReports' warnings and errors, applying the `allowWarnings` logic.
func (v *validator) getErrorsFromGlooValidation(reports []*gloovalidation.GlooValidationReport) error {
	var (
		errs error
	)

	for _, report := range reports {
		err := v.getErrorsFromResourceReports(report.ResourceReports)
		if err != nil {
			errs = multierror.Append(errs, err)
		}

		if proxyReport := report.ProxyReport; proxyReport != nil {
			// Errors always go to errors
			if err := validationutils.GetProxyError(proxyReport); err != nil {
				errs = multierror.Append(errs, errors.Wrapf(err, "getErrorsFromGlooValidation failed to validate Proxy with Gloo validation server"))
			}

			if proxyWarnings := validationutils.GetProxyWarning(proxyReport); !v.allowWarnings && len(proxyWarnings) > 0 {
				for _, warning := range proxyWarnings {
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
