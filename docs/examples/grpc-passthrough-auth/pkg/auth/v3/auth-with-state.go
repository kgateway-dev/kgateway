package v3

import (
	"context"
	"log"

	structpb "github.com/golang/protobuf/ptypes/struct"

	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/genproto/googleapis/rpc/status"
)

const (
	soloPassThroughAuthMetadataKey = "solo.auth.passthrough"
)

type serverWithRequiredState struct {
}

// NewAuthServerWithRequiredState creates a new authorization server
// that authorizes requests based on state passed from other plugins
func NewAuthServerWithRequiredState() envoy_service_auth_v3.AuthorizationServer {
	return &serverWithRequiredState{}
}

// Check implements authorization's Check interface
// Only authorizes requests if they have a jwt token passed from another plugin
func (s *serverWithRequiredState) Check(
	ctx context.Context,
	req *envoy_service_auth_v3.CheckRequest) (*envoy_service_auth_v3.CheckResponse, error) {
	filterMetadata := req.GetAttributes().GetMetadataContext().GetFilterMetadata()
	if filterMetadata == nil {
		log.Println("Request does not have FilterMetadata")
		return unauthorizedResponse()
	}

	// This is the state that is made available to this passthrough service
	availablePassThroughState := filterMetadata[soloPassThroughAuthMetadataKey]

	var jwt string
	if jwtFromState, ok := availablePassThroughState.GetFields()["jwt"]; ok {
		jwt = jwtFromState.GetStringValue()
	}

	if jwt == "" {
		log.Println("Request does not have JWT in FilterMetadata")
		return unauthorizedResponse()
	}

	return &envoy_service_auth_v3.CheckResponse{
		HttpResponse: &envoy_service_auth_v3.CheckResponse_OkResponse{
			OkResponse: &envoy_service_auth_v3.OkHttpResponse{
			},
		},
		Status: &status.Status{
			Code: int32(code.Code_OK),
		},
	}, nil
}

type serverWithNewState struct {
}

// NewAuthServerWithNewState creates a new authorization server
// that authorizes all requests and adds state to be used by other plugins
func NewAuthServerWithNewState() envoy_service_auth_v3.AuthorizationServer {
	return &serverWithNewState{}
}

// Check implements authorization's Check interface
func (s *serverWithNewState) Check(
	ctx context.Context,
	req *envoy_service_auth_v3.CheckRequest) (*envoy_service_auth_v3.CheckResponse, error) {

	// The state you want to make available to other plugins
	newState := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"custom-key": {
				Kind: &structpb.Value_StringValue{
					StringValue: "value",
				},
			},
		},
	}

	return &envoy_service_auth_v3.CheckResponse{
		HttpResponse: &envoy_service_auth_v3.CheckResponse_OkResponse{
			OkResponse: &envoy_service_auth_v3.OkHttpResponse{
			},
		},
		Status: &status.Status{
			Code: int32(code.Code_OK),
		},
		DynamicMetadata: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				soloPassThroughAuthMetadataKey: {
					Kind: &structpb.Value_StructValue{
						StructValue: newState,
					},
				},
			},
		},
	}, nil
}

func unauthorizedResponse() (*envoy_service_auth_v3.CheckResponse, error) {
	return &envoy_service_auth_v3.CheckResponse{
		Status: &status.Status{
			Code: int32(code.Code_PERMISSION_DENIED),
		},
	}, nil
}