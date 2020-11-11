package runner

import (
	"context"
	"fmt"
	"net"

	envoy_data_accesslog_v2 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v2"

	pb "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v2"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/accesslogger/pkg/loggingservice"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/healthchecker"
	"github.com/solo-io/go-utils/stats"
	"go.opencensus.io/plugin/ocgrpc"
	ocstats "go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func init() {
	view.Register(ocgrpc.DefaultServerViews...)
	view.Register(accessLogsRequestsView, accessLogsDownstreamRespTimeView, accessLogsUpstreamRespTimeView)
}

var (
	responseCodeKey, _  = tag.NewKey("response_code")
	clusterKey, _       = tag.NewKey("cluster")
	requestMethodKey, _ = tag.NewKey("request_method")
	// add more keys here (and in the `utils.Measure()` calls) if you want additional dimensions/labels on the
	// access logging metrics. take care to ensure the cardinality of the values of these keys is low enough that
	// prometheus can handle the load.
	tagKeys = []tag.Key{responseCodeKey, clusterKey, requestMethodKey}

	mAccessLogsRequests    = ocstats.Int64("gloo.solo.io/accesslogging/requests", "The number of requests. Can be lossy.", ocstats.UnitDimensionless)
	accessLogsRequestsView = &view.View{
		Name:        "gloo.solo.io/accesslogging/requests",
		Measure:     mAccessLogsRequests,
		Description: "The number of requests. Can be lossy.",
		Aggregation: view.Count(),
		TagKeys:     tagKeys,
	}

	mAccessLogsDownstreamRespTime    = ocstats.Int64("gloo.solo.io/accesslogging/downstream_resp_time", "The downstream request time (ns). Can be lossy.", ocstats.UnitDimensionless)
	accessLogsDownstreamRespTimeView = &view.View{
		Name:        "gloo.solo.io/accesslogging/downstream_resp_time",
		Measure:     mAccessLogsDownstreamRespTime,
		Description: "The downstream request time (ns). Can be lossy.",
		Aggregation: view.Distribution(0.5, 1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 300000, 600000, 1800000, 9000000, 45000000, 225000000, 1125000000, 3375000000),
		TagKeys:     tagKeys,
	}

	mAccessLogsUpstreamRespTime    = ocstats.Int64("gloo.solo.io/accesslogging/upstream_resp_time", "The upstream request time (ns). Can be lossy.", ocstats.UnitDimensionless)
	accessLogsUpstreamRespTimeView = &view.View{
		Name:        "gloo.solo.io/accesslogging/upstream_resp_time",
		Measure:     mAccessLogsUpstreamRespTime,
		Description: "The upstream request time (ns). Can be lossy.",
		Aggregation: view.Distribution(0.5, 1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 300000, 600000, 1800000, 9000000, 45000000, 225000000, 1125000000, 3375000000),
		TagKeys:     tagKeys,
	}
)

func Run() {
	clientSettings := NewSettings()
	ctx := contextutils.WithLogger(context.Background(), "access_log")

	if clientSettings.DebugPort != 0 {
		// TODO(yuval-k): we need to start the stats server before calling contextutils
		// need to think of a better way to express this dependency, or preferably, fix it.
		stats.StartStatsServerWithPort(stats.StartupOptions{Port: clientSettings.DebugPort})
	}

	opts := loggingservice.Options{
		Callbacks: loggingservice.AlsCallbackList{
			func(ctx context.Context, message *pb.StreamAccessLogsMessage) error {
				logger := contextutils.LoggerFrom(ctx)
				switch msg := message.GetLogEntries().(type) {
				case *pb.StreamAccessLogsMessage_HttpLogs:
					for _, v := range msg.HttpLogs.LogEntry {

						meta := v.GetCommonProperties().GetMetadata().GetFilterMetadata()
						// we could put any other kind of data into the transformation metadata, including more
						// detailed request info or info that gets dropped once translated into envoy config. For
						// example, virtual service name, virtual service namespace, virtual service base path,
						// virtual service route (operation path), the request/response body, etc.
						//
						// transformations can live at the virtual host, route, and weighted destination level on the
						// `Proxy`, so users can add very granular information to the transformation filter metadata by
						// configuring transformations on VirtualServices, RouteTables, and/or UpstreamGroups.
						//
						// follow the guide here to create requests with the proper transformation to populate 'pod_name' in the access logs:
						// https://docs.solo.io/gloo-edge/latest/guides/traffic_management/request_processing/transformations/enrich_access_logs/#update-virtual-service
						podName := getTransformationValueFromDynamicMetadata("pod_name", meta)

						// we could change the claim to any other jwt claim, such as client_id
						//
						// follow the guide here to create requests with a jwt that has the 'iss' claim, to populate issuer in the access logs:
						// https://docs.solo.io/gloo-edge/latest/guides/security/auth/jwt/access_control/#appendix---use-a-remote-json-web-key-set-jwks-server
						issuer := getClaimFromJwtInDynamicMetadata("iss", meta)

						utils.MeasureOne(
							ctx,
							mAccessLogsRequests,
							tag.Insert(responseCodeKey, v.GetResponse().GetResponseCode().String()),
							tag.Insert(clusterKey, v.GetCommonProperties().GetUpstreamCluster()),
							tag.Insert(requestMethodKey, v.GetRequest().GetRequestMethod().String()))

						// this includes the time filters take during the processing of the request and response.
						downstreamRespTime := v.GetCommonProperties().GetTimeToLastDownstreamTxByte()
						downstreamRespTimeNs := int64(downstreamRespTime.GetNanos()) + (downstreamRespTime.GetSeconds()*1 ^ 9)

						// if envoy is buffering the request before sending upstream, you want the following
						upstreamRespTimeNs := lastToFirstNs(v)
						// otherwise, you want this
						// upstreamRespTimeNs := firstToFirstNs(v)

						utils.Measure(
							ctx,
							mAccessLogsDownstreamRespTime,
							downstreamRespTimeNs,
							tag.Insert(responseCodeKey, v.GetResponse().GetResponseCode().String()),
							tag.Insert(clusterKey, v.GetCommonProperties().GetUpstreamCluster()),
							tag.Insert(requestMethodKey, v.GetRequest().GetRequestMethod().String()))

						utils.Measure(
							ctx,
							mAccessLogsUpstreamRespTime,
							upstreamRespTimeNs,
							tag.Insert(responseCodeKey, v.GetResponse().GetResponseCode().String()),
							tag.Insert(clusterKey, v.GetCommonProperties().GetUpstreamCluster()),
							tag.Insert(requestMethodKey, v.GetRequest().GetRequestMethod().String()))

						logger.With(
							zap.Any("protocol_version", v.GetProtocolVersion()),
							zap.Any("request_path", v.GetRequest().GetPath()),
							zap.Any("request_original_path", v.GetRequest().GetOriginalPath()),
							zap.Any("request_method", v.GetRequest().GetRequestMethod().String()),
							zap.Any("response_code", v.GetResponse().GetResponseCode().String()),
							zap.Any("cluster", v.GetCommonProperties().GetUpstreamCluster()),
							zap.Any("upstream_remote_address", v.GetCommonProperties().GetUpstreamRemoteAddress()),
							zap.Any("issuer", issuer),                                     // requires jwt set up and jwt with 'iss' claim to be non-empty
							zap.Any("pod_name", podName),                                  // requires transformation set up with dynamic metadata (with 'pod_name' key) to be non-empty
							zap.Any("route_name", v.GetCommonProperties().GetRouteName()), // empty by default, but name can be set on routes in virtual services or route tables
							zap.Any("start_time", v.GetCommonProperties().GetStartTime()),
							zap.Any("downstream_resp_time", downstreamRespTimeNs),
							zap.Any("upstream_resp_time", upstreamRespTimeNs),
						).Info("received http request")
					}
				case *pb.StreamAccessLogsMessage_TcpLogs:
					for _, v := range msg.TcpLogs.LogEntry {
						logger.With(
							zap.Any("upstream_cluster", v.GetCommonProperties().GetUpstreamCluster()),
							zap.Any("route_name", v.GetCommonProperties().GetRouteName()),
						).Info("received tcp request")
					}
				}
				return nil
			},
		},
		Ctx: ctx,
	}
	service := loggingservice.NewServer(opts)

	err := RunWithSettings(ctx, service, clientSettings)

	if err != nil {
		if ctx.Err() == nil {
			// not a context error - panic
			panic(err)
		}
	}
}

func RunWithSettings(ctx context.Context, service *loggingservice.Server, clientSettings Settings) error {
	err := StartAccessLog(ctx, clientSettings, service)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func StartAccessLog(ctx context.Context, clientSettings Settings, service *loggingservice.Server) error {
	srv := grpc.NewServer(grpc.StatsHandler(&ocgrpc.ServerHandler{}))

	pb.RegisterAccessLogServiceServer(srv, service)
	hc := healthchecker.NewGrpc(clientSettings.ServiceName, health.NewServer())
	healthpb.RegisterHealthServer(srv, hc.GetServer())
	reflection.Register(srv)

	logger := contextutils.LoggerFrom(ctx)
	logger.Infow("Starting access-log server")

	addr := fmt.Sprintf(":%d", clientSettings.ServerPort)
	runMode := "gRPC"
	network := "tcp"

	logger.Infof("access-log server running in [%s] mode, listening at [%s]", runMode, addr)
	lis, err := net.Listen(network, addr)
	if err != nil {
		logger.Errorw("Failed to announce on network", zap.Any("mode", runMode), zap.Any("address", addr), zap.Any("error", err))
		return err
	}
	go func() {
		<-ctx.Done()
		srv.Stop()
		_ = lis.Close()
	}()

	return srv.Serve(lis)
}

func getTransformationValueFromDynamicMetadata(key string, filterMetadata map[string]*_struct.Struct) string {
	transformationMeta := filterMetadata[transformation.FilterName]
	for tKey, tVal := range transformationMeta.GetFields() {
		if tKey == key {
			return tVal.GetStringValue()
		}
	}
	return ""
}

func getClaimFromJwtInDynamicMetadata(claim string, filterMetadata map[string]*_struct.Struct) string {
	providerByJwt := filterMetadata["envoy.filters.http.jwt_authn"]
	jwts := providerByJwt.GetFields()
	for _, jwt := range jwts {
		claims := jwt.GetStructValue()
		if claims != nil {
			claimsMap := claims.GetFields()
			if val, ok := claimsMap[claim]; ok {
				return val.GetStringValue()
			}
		}
	}
	return ""
}

func firstToFirstNs(entry *envoy_data_accesslog_v2.HTTPAccessLogEntry) int64 {
	timeToFirstUpstreamRxByte := entry.GetCommonProperties().GetTimeToFirstUpstreamRxByte()
	timeToFirstUpstreamRxByteNs := int64(timeToFirstUpstreamRxByte.GetNanos()) + (timeToFirstUpstreamRxByte.GetSeconds()*1 ^ 9)
	timeToFirstUpstreamTxByte := entry.GetCommonProperties().GetTimeToFirstUpstreamTxByte()
	timeToFirstUpstreamTxByteNs := int64(timeToFirstUpstreamTxByte.GetNanos()) + (timeToFirstUpstreamTxByte.GetSeconds()*1 ^ 9)

	// this excludes the time filters take during the processing of the request and response.
	upstreamRespTimeNs := timeToFirstUpstreamRxByteNs - timeToFirstUpstreamTxByteNs
	return upstreamRespTimeNs
}

func lastToFirstNs(entry *envoy_data_accesslog_v2.HTTPAccessLogEntry) int64 {
	timeToFirstUpstreamRxByte := entry.GetCommonProperties().GetTimeToFirstUpstreamRxByte()
	timeToFirstUpstreamRxByteNs := int64(timeToFirstUpstreamRxByte.GetNanos()) + (timeToFirstUpstreamRxByte.GetSeconds()*1 ^ 9)
	timeToLastUpstreamTxByte := entry.GetCommonProperties().GetTimeToLastUpstreamTxByte()
	timeToLastUpstreamTxByteNs := int64(timeToLastUpstreamTxByte.GetNanos()) + (timeToLastUpstreamTxByte.GetSeconds()*1 ^ 9)

	// this excludes the time filters take during the processing of the request and response.
	// this could, in theory, be negative. for example, the upstream could reject based on the
	// request headers and respond before the request body had finished transmitting upstream.
	upstreamRespTimeNs := timeToFirstUpstreamRxByteNs - timeToLastUpstreamTxByteNs
	return upstreamRespTimeNs
}
