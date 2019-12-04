package validation

import (
	"context"
	"sync"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
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
	notifyResync   map[*validation.NotificationRequest]chan struct{}
}

func NewValidator(translator translator.Translator) *validator {
	return &validator{translator: translator, notifyResync: make(map[*validation.NotificationRequest]chan struct{}, 1)}
}

func (s *validator) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	snapCopy := snap.Clone()
	s.lock.Lock()
	s.latestSnapshot = &snapCopy
	// notify all receivers
	for _, receiver := range s.notifyResync {
		receiver := receiver
		go func() {
			select {
			// only write to channel if it's empty
			case receiver <- struct{}{}:
			default:
			}
		}()
	}
	s.lock.Unlock()
	return nil
}

func (s *validator) NotifyOnResync(req *validation.NotificationRequest, stream validation.ProxyValidationService_NotifyOnResyncServer) error {
	// send initial response as ACK
	if err := stream.Send(&validation.NotificationResponse{}); err != nil {
		return err
	}

	// initialize a receiver. this will receive all update notifications
	// size of one so we don't queue multiple notifications
	receiver := make(chan struct{}, 1)

	// add the receiver to our map
	s.lock.Lock()
	s.notifyResync[req] = receiver
	s.lock.Unlock()

	defer func() {
		// remove the receiver from the map
		s.lock.Lock()
		delete(s.notifyResync, req)
		s.lock.Unlock()
	}()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-receiver:
			if err := stream.Send(&validation.NotificationResponse{}); err != nil {
				contextutils.LoggerFrom(stream.Context()).Errorw("failed to send validation resync notification", zap.Error(err))
			}
		}
	}
}

func (s *validator) ValidateProxy(ctx context.Context, req *validation.ProxyValidationServiceRequest) (*validation.ProxyValidationServiceResponse, error) {
	s.lock.RLock()
	snapCopy := s.latestSnapshot.Clone()
	s.lock.RUnlock()

	ctx = contextutils.WithLogger(ctx, "proxy-validator")

	params := plugins.Params{Ctx: ctx, Snapshot: &snapCopy}

	logger := contextutils.LoggerFrom(ctx)

	logger.Infof("received proxy validation request")
	_, _, report, err := s.translator.Translate(params, req.GetProxy())
	if err != nil {
		logger.Errorw("failed to validate proxy", zap.Error(err))
		return nil, err
	}
	logger.Infof("proxy validation report result: %v", report.String())
	return &validation.ProxyValidationServiceResponse{ProxyReport: report}, nil
}

type ValidationServer interface {
	validation.ProxyValidationServiceServer
	SetValidator(v Validator)
	Register(grpcServer *grpc.Server)
}

type validationServer struct {
	lock      sync.Mutex
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

func (s *validationServer) NotifyOnResync(req *validation.NotificationRequest, stream validation.ProxyValidationService_NotifyOnResyncServer) error {
	s.lock.Lock()
	validator := s.validator
	s.lock.Unlock()

	return validator.NotifyOnResync(req, stream)
}

func (s *validationServer) ValidateProxy(ctx context.Context, req *validation.ProxyValidationServiceRequest) (*validation.ProxyValidationServiceResponse, error) {
	s.lock.Lock()
	validator := s.validator
	s.lock.Unlock()

	return validator.ValidateProxy(ctx, req)
}
