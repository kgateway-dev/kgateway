package validation

import (
	"context"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"go.uber.org/zap/zapcore"

	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/hashutils"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/sanitizer"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"google.golang.org/grpc"
)

type Validator interface {
	v1.ApiSyncer
	validation.ProxyValidationServiceServer
}

type validator struct {
	lock           sync.RWMutex
	latestSnapshot *v1.ApiSnapshot
	translator     translator.Translator
	notifyResync   map[*validation.NotifyOnResyncRequest]chan struct{}
	ctx            context.Context
	xdsSanitizer   sanitizer.XdsSanitizers
}

func NewValidator(ctx context.Context, translator translator.Translator, xdsSanitizer sanitizer.XdsSanitizers) *validator {
	return &validator{
		translator:   translator,
		notifyResync: make(map[*validation.NotifyOnResyncRequest]chan struct{}, 1),
		ctx:          ctx,
		xdsSanitizer: xdsSanitizer,
	}
}

// only call within a lock
// should we notify on this snap update
func (s *validator) shouldNotify(snap *v1.ApiSnapshot) bool {
	if s.latestSnapshot == nil {
		return true
	}
	// rather than compare the hash of the whole snapshot,
	// we compare the hash of resources that can affect
	// the validation result (which excludes Endpoints)
	hashFunc := func(snap *v1.ApiSnapshot) uint64 {
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
			panic("this error should never happen, as this is safe hasher")
		}
		return hash
	}

	hashChanged := hashFunc(s.latestSnapshot) != hashFunc(snap)

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
func (s *validator) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	snapCopy := snap.Clone()
	s.lock.Lock()
	if s.shouldNotify(snap) {
		s.pushNotifications()
	}
	s.latestSnapshot = &snapCopy
	s.lock.Unlock()
	return nil
}

func (s *validator) NotifyOnResync(req *validation.NotifyOnResyncRequest, stream validation.ProxyValidationService_NotifyOnResyncServer) error {
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

func (s *validator) ValidateProxy(ctx context.Context, req *validation.ProxyValidationServiceRequest) (*validation.ProxyValidationServiceResponse, error) {
	s.lock.RLock()
	// we may receive a ValidateProxy call before a Sync has occurred
	if s.latestSnapshot == nil {
		s.lock.RUnlock()
		return nil, eris.New("proxy validation called before the validation server received its first sync of resources")
	}
	snapCopy := s.latestSnapshot.Clone()
	s.lock.RUnlock()

	ctx = contextutils.WithLogger(ctx, "proxy-validator")

	params := plugins.Params{Ctx: ctx, Snapshot: &snapCopy}

	logger := contextutils.LoggerFrom(ctx)

	logger.Infof("received proxy validation request")
	xdsSnapshot, resourceReports, report, err := s.translator.Translate(params, req.GetProxy())
	if err != nil {
		logger.Errorw("failed to validate proxy", zap.Error(err))
		return nil, err
	}

	// Sanitize routes before sending report to gateway
	s.xdsSanitizer.SanitizeSnapshot(ctx, &snapCopy, xdsSnapshot, resourceReports)
	routeErrorToWarnings(resourceReports, report)

	logger.Infof("proxy validation report result: %v", report.String())
	return &validation.ProxyValidationServiceResponse{ProxyReport: report}, nil
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
		for _, virtualHostReport := range listenerReport.GetHttpListenerReport().GetVirtualHostReports() {
			for _, routeReport := range virtualHostReport.GetRouteReports() {
				modifiedErrors := make([]*validation.RouteReport_Error, 0)
				for _, rError := range routeReport.GetErrors() {
					if _, inErrors := resourceReportErrors[rError.String()]; !inErrors {
						if _, inWarnings := resourceReportWarnings[rError.String()]; inWarnings {
							warning := &validation.RouteReport_Warning{
								Type:   validation.RouteReport_Warning_Type(rError.GetType()),
								Reason: rError.GetReason(),
							}
							routeReport.Warnings = append(routeReport.Warnings, warning)
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
	validation.ProxyValidationServiceServer
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
	validation.RegisterProxyValidationServiceServer(grpcServer, s)
}

func (s *validationServer) NotifyOnResync(req *validation.NotifyOnResyncRequest, stream validation.ProxyValidationService_NotifyOnResyncServer) error {
	s.lock.RLock()
	validator := s.validator
	s.lock.RUnlock()

	return validator.NotifyOnResync(req, stream)
}

func (s *validationServer) ValidateProxy(ctx context.Context, req *validation.ProxyValidationServiceRequest) (*validation.ProxyValidationServiceResponse, error) {
	s.lock.RLock()
	validator := s.validator
	s.lock.RUnlock()

	return validator.ValidateProxy(ctx, req)
}
