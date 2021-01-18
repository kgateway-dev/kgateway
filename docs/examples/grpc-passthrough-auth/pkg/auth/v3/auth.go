package v3

import (
	"context"
	"log"
	"strings"

	envoy_api_v3_core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes/wrappers"

	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
)

type server struct {
}

var _ envoy_service_auth_v3.AuthorizationServer = &server{}

// New creates a new authorization server.
func New() envoy_service_auth_v3.AuthorizationServer {
	return &server{}
}

// Check implements authorization's Check interface which performs authorization check based on the
// attributes associated with the incoming request.
func (s *server) Check(
	ctx context.Context,
	req *envoy_service_auth_v3.CheckRequest) (*envoy_service_auth_v3.CheckResponse, error) {
	authorization := req.Attributes.Request.Http.Headers["authorization"]
	if strings.Contains(authorization, "authorize me") {
		log.Println("Recieved request with correct authorization header")
		return &envoy_service_auth_v3.CheckResponse{
			HttpResponse: &envoy_service_auth_v3.CheckResponse_OkResponse{
				OkResponse: &envoy_service_auth_v3.OkHttpResponse{
					Headers: []*envoy_api_v3_core.HeaderValueOption{
						{
							Append: &wrappers.BoolValue{Value: false},
							Header: &envoy_api_v3_core.HeaderValue{
								// For a successful request, the authorization server sets the
								// x-current-user value.
								Key:   "x-authorized",
								Value: "you are authorized",
							},
						},
					},
				},
			},
			Status: &status.Status{
				Code: int32(code.Code_OK),
			},
		}, nil
	}
	log.Println("Request does not have correct authorization header")
	return &envoy_service_auth_v3.CheckResponse{
		Status: &status.Status{
			Code: int32(code.Code_PERMISSION_DENIED),
		},
	}, nil
}
