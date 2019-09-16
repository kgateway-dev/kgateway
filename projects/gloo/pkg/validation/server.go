package validation

import (
	"context"
	"sync"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"google.golang.org/grpc"
)

type ValidationServer interface {
	v1.ApiSyncer
	validation.ProxyValidationServiceServer
	Register(grpcServer *grpc.Server)
}

type validationServer struct {
	l              sync.RWMutex
	latestSnapshot *v1.ApiSnapshot
	translator     translator.Translator
}

func (s *validationServer) ValidateProxy(ctx context.Context, req *validation.ProxyValidationServiceRequest) (*validation.ProxyValidationServiceResponse, error) {
	s.l.RLock()
	snapCopy := s.latestSnapshot.Clone()
	s.l.RUnlock()

	params := plugins.Params{Ctx: ctx, Snapshot: &snapCopy}

	_, _, report, err := s.translator.Translate(params, req.GetProxy())
	if err != nil {
		return nil, err
	}
	return &validation.ProxyValidationServiceResponse{ProxyReport: report}, nil
}

func NewValidationServer(translator translator.Translator) ValidationServer {
	return &validationServer{translator: translator}
}

func (s *validationServer) Sync(_ context.Context, snap *v1.ApiSnapshot) error {
	snapCopy := snap.Clone()
	s.l.Lock()
	s.latestSnapshot = &snapCopy
	s.l.Unlock()
	return nil
}

func (s *validationServer) Register(grpcServer *grpc.Server) {
	validation.RegisterProxyValidationServiceServer(grpcServer, s)
}
