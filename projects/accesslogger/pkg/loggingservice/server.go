package loggingservice

import (
	"context"

	envoyals "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// server is used to implement envoyals.AccessLogServiceServer.

type AlsCallback func(ctx context.Context, message *envoyals.StreamAccessLogsMessage) error
type AlsCallbackList []AlsCallback

type Server struct {
	opts *Options
}

var _ envoyals.AccessLogServiceServer = new(Server)

func (s *Server) StreamAccessLogs(srv envoyals.AccessLogService_StreamAccessLogsServer) error {
	msg, err := srv.Recv()
	if err != nil {
		return err
	}

	ctx := contextutils.WithLoggerValues(
		s.opts.Ctx,
		zap.String("logger_name", msg.GetIdentifier().GetLogName()),
		zap.String("node_id", msg.GetIdentifier().GetNode().GetId()),
		zap.String("node_cluster", msg.GetIdentifier().GetNode().GetCluster()),
		zap.Any("node_locality", msg.GetIdentifier().GetNode().GetLocality()),
		zap.Any("node_metadata", msg.GetIdentifier().GetNode().GetMetadata()),
	)
	contextutils.LoggerFrom(ctx).Info("received access log message")

	if s.opts.Ordered {
		for _, cb := range s.opts.Callbacks {
			if err := cb(ctx, msg); err != nil {
				return err
			}
		}
	} else {
		eg := errgroup.Group{}
		for _, cb := range s.opts.Callbacks {
			cb := cb
			eg.Go(func() error {
				return cb(ctx, msg)
			})
		}
		if err := eg.Wait(); err != nil {
			return err
		}
	}
	return nil
}

type Options struct {
	Ordered   bool
	Callbacks AlsCallbackList
	Ctx       context.Context
}

func NewServer(opts Options) *Server {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}
	return &Server{opts: &opts}
}
