package validation

import (
	"context"
	"errors"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	sk_resources "github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
)

type Validator interface {
	v1snap.ApiSyncer
	validation.GlooValidationServiceServer
	ValidateGloo(ctx context.Context, proxy *v1.Proxy, resource resources.Resource, delete bool) ([]*GlooValidationReport, error)
}

// ValidatorConfig is used to configure the validator
type ValidatorConfig struct {
	Ctx                 context.Context
	GlooValidatorConfig GlooValidatorConfig
}

type validator struct {
	// note to devs: this can be called in parallel by the validation webhook and main translation loops at the same time
	// any stateful fields should be protected by a mutex or themselves be synchronized (like the xds sanitizer / translator)
	lock           sync.RWMutex
	latestSnapshot *v1snap.ApiSnapshot
	validator      GlooValidator
	translator     translator.Translator
	notifyResync   map[*validation.NotifyOnResyncRequest]chan struct{}
	ctx            context.Context
}

func NewValidator(config ValidatorConfig) *validator {
	return &validator{
		translator:   config.GlooValidatorConfig.Translator,
		validator:    NewGlooValidator(config.GlooValidatorConfig),
		notifyResync: make(map[*validation.NotifyOnResyncRequest]chan struct{}, 1),
		ctx:          config.Ctx,
	}
}

// only call within a lock
// should we notify on this snap update
func (s *validator) shouldNotify(snap *v1snap.ApiSnapshot) bool {
	if s.latestSnapshot == nil {
		return true
	}
	// rather than compare the hash of the whole snapshot,
	// we compare the hash of resources that can affect
	// the validation result (which excludes Endpoints)
	hashFunc := func(snap *v1snap.ApiSnapshot) (uint64, error) {
		toHash := append([]interface{}{}, snap.Upstreams.AsInterfaces()...)
		toHash = append(toHash, snap.UpstreamGroups.AsInterfaces()...)
		toHash = append(toHash, snap.Secrets.AsInterfaces()...)
		toHash = append(toHash, snap.AuthConfigs.AsInterfaces()...)
		toHash = append(toHash, snap.Ratelimitconfigs.AsInterfaces()...)
		// we also include proxies as this will help
		// the gateway to resync in case the proxy was deleted
		toHash = append(toHash, snap.Proxies.AsInterfaces()...)

		hash, err := hashutils.HashAllSafe(nil, toHash...)
		if err != nil {
			contextutils.LoggerFrom(context.Background()).DPanic("this error should never happen, as this is safe hasher")
			return 0, errors.New("this error should never happen, as this is safe hasher")
		}
		return hash, nil
	}
	oldHash, oldHashErr := hashFunc(s.latestSnapshot)
	newHash, newHashErr := hashFunc(snap)
	// If we cannot hash then we choose to treat them as different hashes since this is just a performance optimization.
	// In worst case we'd prefer correctness
	hashChanged := oldHash != newHash || oldHashErr != nil || newHashErr != nil

	logger := contextutils.LoggerFrom(s.ctx)
	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debugw("last validation snapshot", zap.Any("latestSnapshot", syncutil.StringifySnapshot(s.latestSnapshot)))
		logger.Debugw("current validation snapshot", zap.Any("currentSnapshot", syncutil.StringifySnapshot(snap)))
	}
	logger.Debugf("validation hash changed: %v", hashChanged)

	// notify if the hash of what we care about has changed
	return hashChanged
}

// only call within a lock
// notify all receivers
func (s *validator) pushNotifications() {
	logger := contextutils.LoggerFrom(s.ctx)
	logger.Debugw("pushing notifications", zap.Any("validator", s))
	for _, receiver := range s.notifyResync {
		logger.Debugf("pushing notification for receiver %v", receiver)
		receiver := receiver
		go func() {
			select {
			// only write to channel if it's empty
			case receiver <- struct{}{}:
			default:
			}
		}()
	}
}

// the gloo snapshot has changed.
// update the local snapshot, notify subscribers
func (s *validator) Sync(ctx context.Context, snap *v1snap.ApiSnapshot) error {
	snapCopy := snap.Clone()
	s.lock.Lock()
	if s.shouldNotify(snap) {
		s.pushNotifications()
	}
	s.latestSnapshot = &snapCopy
	s.lock.Unlock()
	return nil
}

func (s *validator) NotifyOnResync(req *validation.NotifyOnResyncRequest, stream validation.GlooValidationService_NotifyOnResyncServer) error {
	// send initial response as ACK
	if err := stream.Send(&validation.NotifyOnResyncResponse{}); err != nil {
		return err
	}

	// initialize a receiver. this will receive all update notifications
	// size of one so we don't queue multiple notifications
	receiver := make(chan struct{}, 1)

	logger := contextutils.LoggerFrom(s.ctx)

	// add the receiver to our map
	s.lock.Lock()
	s.notifyResync[req] = receiver
	logger.Debug("added receiver to map", zap.Any("newReceiver", receiver), zap.Any("afterMap", s.notifyResync), zap.Any("validator", s))
	s.lock.Unlock()

	defer func() {
		// remove the receiver from the map
		s.lock.Lock()
		delete(s.notifyResync, req)
		logger.Debug("removed receiver from map", zap.Any("removedReceiver", req), zap.Any("afterMap", s.notifyResync), zap.Any("validator", s))
		s.lock.Unlock()
	}()

	// loop forever, sending a notification
	// whenever we read from the receiver channel
	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-receiver:
			if err := stream.Send(&validation.NotifyOnResyncResponse{}); err != nil {
				contextutils.LoggerFrom(stream.Context()).Errorw("failed to send validation resync notification", zap.Error(err))
			}
		}
	}
}

// Validate is a gRPC call that we use for validating resources against a request to add upstreams and secrets.
func (s *validator) Validate(ctx context.Context, req *validation.GlooValidationServiceRequest) (*validation.GlooValidationServiceResponse, error) {
	s.lock.Lock()
	// we may receive a Validate call before a Sync has occurred
	if s.latestSnapshot == nil {
		s.lock.Unlock()
		return nil, eris.New("proxy validation called before the validation server received its first sync of resources")
	}
	snapCopy := s.latestSnapshot.Clone() // cloning can mutate so we need a write lock
	s.lock.Unlock()

	// update the snapshot copy with the resources from the request
	applyRequestToSnapshot(&snapCopy, req)
	contextutils.LoggerFrom(ctx).Infof("received proxy validation request")

	reports := s.validator.Validate(ctx, req.GetProxy(), &snapCopy, false)

	var validationReports []*validation.ValidationReport
	// convert the reports for the gRPC response
	for _, rep := range reports {
		validationReports = append(validationReports, convertToValidationReport(rep.ProxyReport, rep.ResourceReports, rep.Proxy))
	}
	return &validation.GlooValidationServiceResponse{
		ValidationReports: validationReports,
	}, nil
}

// ValidateGloo replaces the functionality of Validate.  Validate is still a method that needs to be
// exported because it is used as a gRPC service. A synced version of the snapshot is needed for
// gloo validation.
func (s *validator) ValidateGloo(ctx context.Context, proxy *v1.Proxy, resource resources.Resource, delete bool) ([]*GlooValidationReport, error) {
	// the gateway validator will call this function to validate Gloo resources.
	s.lock.Lock()
	// we may receive a Validate call before a Sync has occurred
	if s.latestSnapshot == nil {
		s.lock.Unlock()
		return nil, eris.New("proxy validation called before the validation server received its first sync of resources")
	}
	snapCopy := s.latestSnapshot.Clone() // cloning can mutate so we need a write lock
	s.lock.Unlock()
	if resource != nil {
		if delete {
			if err := snapCopy.RemoveFromResourceList(resource); err != nil {
				return nil, err
			}
		} else {
			if err := snapCopy.UpsertToResourceList(resource); err != nil {
				return nil, err
			}
		}
	}

	return s.validator.Validate(ctx, proxy, &snapCopy, delete), nil
}

// updates the given snapshot with the resources from the request
func applyRequestToSnapshot(snap *v1snap.ApiSnapshot, req *validation.GlooValidationServiceRequest) {
	// if we want to change the type, we could use API snapshots as containers.  Like this struct
	// projects/gloo/pkg/validation/api_snapshot_request.go
	if req.GetModifiedResources() != nil {
		existingUpstreams := snap.Upstreams.AsResources()
		modifiedUpstreams := utils.UpstreamsToResourceList(req.GetModifiedResources().GetUpstreams())
		mergedUpstreams := utils.MergeResourceLists(existingUpstreams, modifiedUpstreams)
		snap.Upstreams = utils.ResourceListToUpstreamList(mergedUpstreams)
	} else if req.GetDeletedResources() != nil {
		// Upstreams
		existingUpstreams := snap.Upstreams.AsResources()
		deletedUpstreamRefs := req.GetDeletedResources().GetUpstreamRefs()
		finalUpstreams := utils.DeleteResources(existingUpstreams, deletedUpstreamRefs)
		snap.Upstreams = utils.ResourceListToUpstreamList(finalUpstreams)
		// Secrets
		existingSecrets := snap.Secrets.AsResources()
		deletedSecretRefs := req.GetDeletedResources().GetSecretRefs()
		finalSecrets := utils.DeleteResources(existingSecrets, deletedSecretRefs)
		snap.Secrets = utils.ResourceListToSecretList(finalSecrets)
	}
}

func convertToValidationReport(proxyReport *validation.ProxyReport, resourceReports reporter.ResourceReports, proxy *v1.Proxy) *validation.ValidationReport {
	var upstreamReports []*validation.ResourceReport

	for resource, report := range resourceReports {
		switch sk_resources.Kind(resource) {
		case "*v1.Upstream":
			upstreamReports = append(upstreamReports, &validation.ResourceReport{
				ResourceRef: resource.GetMetadata().Ref(),
				Warnings:    report.Warnings,
				Errors:      getErrors(report.Errors),
			})
		}
		// TODO add other resources types here
	}

	return &validation.ValidationReport{
		ProxyReport:     proxyReport,
		UpstreamReports: upstreamReports,
		Proxy:           proxy,
	}
}

func getErrors(err error) []string {
	if err == nil {
		return []string{}
	}
	switch err.(type) {
	case *multierror.Error:
		var errorStrings []string
		for _, e := range err.(*multierror.Error).Errors {
			errorStrings = append(errorStrings, e.Error())
		}
		return errorStrings
	}
	return []string{err.Error()}
}

// Update the validation report so that route errors that were changed into warnings during sanitization
// are also switched in the report results
func routeErrorToWarnings(resourceReport reporter.ResourceReports, validationReport *validation.ProxyReport) {
	// Only proxy reports are needed
	resourceReport = resourceReport.FilterByKind("*v1.Proxy")
	resourceReportErrors := make(map[string]struct{})
	resourceReportWarnings := make(map[string]struct{})
	for _, report := range resourceReport {
		if report.Errors != nil {
			for _, rError := range report.Errors.(*multierror.Error).Errors {
				resourceReportErrors[rError.Error()] = struct{}{}
			}
		}

		for _, rWarning := range report.Warnings {
			resourceReportWarnings[rWarning] = struct{}{}
		}
	}

	for _, listenerReport := range validationReport.GetListenerReports() {
		virtualHostReports := utils.GetVhostReportsFromListenerReport(listenerReport)

		for _, virtualHostReport := range virtualHostReports {
			for _, routeReport := range virtualHostReport.GetRouteReports() {
				modifiedErrors := make([]*validation.RouteReport_Error, 0)
				for _, rError := range routeReport.GetErrors() {
					if _, inErrors := resourceReportErrors[rError.String()]; !inErrors {
						if _, inWarnings := resourceReportWarnings[rError.String()]; inWarnings {
							warning := &validation.RouteReport_Warning{
								Type:   validation.RouteReport_Warning_Type(rError.GetType()),
								Reason: rError.GetReason(),
							}
							routeReport.Warnings = append(routeReport.GetWarnings(), warning)
						}
					} else {
						modifiedErrors = append(modifiedErrors, rError)
					}
				}
				routeReport.Errors = modifiedErrors
			}
		}
	}
}

type ValidationServer interface {
	validation.GlooValidationServiceServer
	SetValidator(v Validator)
	Register(grpcServer *grpc.Server)
}

type validationServer struct {
	lock      sync.RWMutex
	validator Validator
}

func NewValidationServer() *validationServer {
	return &validationServer{}
}

func (s *validationServer) SetValidator(v Validator) {
	s.lock.Lock()
	s.validator = v
	s.lock.Unlock()
}

func (s *validationServer) Register(grpcServer *grpc.Server) {
	validation.RegisterGlooValidationServiceServer(grpcServer, s)
}

func (s *validationServer) NotifyOnResync(req *validation.NotifyOnResyncRequest, stream validation.GlooValidationService_NotifyOnResyncServer) error {
	s.lock.RLock()
	validator := s.validator
	s.lock.RUnlock()

	return validator.NotifyOnResync(req, stream)
}

func (s *validationServer) Validate(ctx context.Context, req *validation.GlooValidationServiceRequest) (*validation.GlooValidationServiceResponse, error) {
	s.lock.RLock()
	validator := s.validator
	s.lock.RUnlock()

	return validator.Validate(ctx, req)
}
