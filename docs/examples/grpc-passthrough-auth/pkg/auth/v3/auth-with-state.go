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

type serverWithRequiredJwtToken struct {
}

// NewAuthServerWithRequiredJwtToken creates a new authorization server
// that authorizes requests based on whether a JWT token was extracted in an earlier stage
func NewAuthServerWithRequiredJwtToken() envoy_service_auth_v3.AuthorizationServer {
	return &serverWithRequiredJwtToken{}
}

// Check implements authorization's Check interface
// Only authorizes requests if they have a successfully extracted a JWT ID token
// from an OIDC flow
func (s *serverWithRequiredJwtToken) Check(
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
	// During our build-in OIDC flow, the `jwt` field is set with the contents of the JWT ID token
	// We can access that value, and apply some custom logic logic to that token, though in this instance
	// we are only checking its presence
	if jwtFromState, ok := availablePassThroughState.GetFields()["jwt"]; ok {
		jwt = jwtFromState.GetStringValue()
	}

	if jwt == "" {
		log.Println("Request does not have JWT in FilterMetadata")
		return unauthorizedResponse()
	}

	return &envoy_service_auth_v3.CheckResponse{
		HttpResponse: &envoy_service_auth_v3.CheckResponse_OkResponse{
			OkResponse: &envoy_service_auth_v3.OkHttpResponse{},
		},
		Status: &status.Status{
			Code: int32(code.Code_OK),
		},
	}, nil
}

type serverWithNewState struct {
}

// NewAuthServerWithNewState creates a new authorization server
// that authorizes all requests and adds state to be used by other auth steps
func NewAuthServerWithNewState() envoy_service_auth_v3.AuthorizationServer {
	return &serverWithNewState{}
}

// Check implements authorization's Check interface
func (s *serverWithNewState) Check(
	ctx context.Context,
	req *envoy_service_auth_v3.CheckRequest) (*envoy_service_auth_v3.CheckResponse, error) {

	// The state you want to make available to other auth steps
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
			OkResponse: &envoy_service_auth_v3.OkHttpResponse{},
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
