package main

import (
	"context"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestHasJWTClaim(t *testing.T) {
	tests := []struct {
		name        string
		metadata    map[string]*structpb.Struct
		namespace   string
		claimKey    string
		claimValue  string
		expectedHit bool
	}{
		{
			name:        "verified payload claim matches",
			metadata:    jwtMetadata("email", "dev1@kgateway.io"),
			namespace:   defaultMetadataNamespace,
			claimKey:    "email",
			claimValue:  "dev1@kgateway.io",
			expectedHit: true,
		},
		{
			name:        "wrong claim value is rejected",
			metadata:    jwtMetadata("email", "dev2@kgateway.io"),
			namespace:   defaultMetadataNamespace,
			claimKey:    "email",
			claimValue:  "dev1@kgateway.io",
			expectedHit: false,
		},
		{
			name:        "wrong claim key is rejected",
			metadata:    jwtMetadata("sub", "dev1@kgateway.io"),
			namespace:   defaultMetadataNamespace,
			claimKey:    "email",
			claimValue:  "dev1@kgateway.io",
			expectedHit: false,
		},
		{
			name:        "claim outside payload is rejected",
			metadata:    claimOutsidePayloadMetadata("email", "dev1@kgateway.io"),
			namespace:   defaultMetadataNamespace,
			claimKey:    "email",
			claimValue:  "dev1@kgateway.io",
			expectedHit: false,
		},
		{
			name:        "missing namespace is rejected",
			metadata:    jwtMetadata("email", "dev1@kgateway.io"),
			namespace:   "envoy.filters.http.some_other_filter",
			claimKey:    "email",
			claimValue:  "dev1@kgateway.io",
			expectedHit: false,
		},
		{
			name:        "empty expected value never matches",
			metadata:    jwtMetadata("email", ""),
			namespace:   defaultMetadataNamespace,
			claimKey:    "email",
			claimValue:  "",
			expectedHit: false,
		},
		{
			name:        "nil metadata is rejected",
			metadata:    nil,
			namespace:   defaultMetadataNamespace,
			claimKey:    "email",
			claimValue:  "dev1@kgateway.io",
			expectedHit: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasJWTClaim(tc.metadata, tc.namespace, tc.claimKey, tc.claimValue)
			if got != tc.expectedHit {
				t.Fatalf("hasJWTClaim() = %v, want %v", got, tc.expectedHit)
			}
		})
	}
}

func TestCheckUsesJWTMetadata(t *testing.T) {
	originalNamespace, originalKey, originalValue := *metadataNamespace, *claimKey, *claimValue
	originalSkip, originalHeader := *skipJWTMetadata, *requiredHeader
	defer func() {
		*metadataNamespace, *claimKey, *claimValue = originalNamespace, originalKey, originalValue
		*skipJWTMetadata, *requiredHeader = originalSkip, originalHeader
	}()
	*metadataNamespace, *claimKey, *claimValue = defaultMetadataNamespace, "email", "dev1@kgateway.io"
	*skipJWTMetadata, *requiredHeader = false, ""

	server := &authorizationServer{}
	tests := []struct {
		name           string
		metadata       map[string]*structpb.Struct
		expectedCode   int32
		expectedHeader bool
	}{
		{
			name:           "matching payload is allowed with marker header",
			metadata:       jwtMetadata("email", "dev1@kgateway.io"),
			expectedCode:   int32(codes.OK),
			expectedHeader: true,
		},
		{
			name:           "different payload is denied",
			metadata:       jwtMetadata("email", "dev2@kgateway.io"),
			expectedCode:   int32(codes.PermissionDenied),
			expectedHeader: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := server.Check(context.Background(), &authv3.CheckRequest{
				Attributes: &authv3.AttributeContext{
					MetadataContext: &envoycorev3.Metadata{FilterMetadata: tc.metadata},
				},
			})
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got := resp.GetStatus().GetCode(); got != tc.expectedCode {
				t.Fatalf("Check() status = %v, want %v", got, tc.expectedCode)
			}

			okResp := resp.GetOkResponse()
			if tc.expectedHeader {
				if okResp == nil || len(okResp.GetHeaders()) != 1 {
					t.Fatalf("Check() ok response headers = %v, want one header", okResp)
				}
				header := okResp.GetHeaders()[0].GetHeader()
				// Envoy reads HeaderValue.value unless read_raw_header_value is enabled.
				if header.GetKey() != allowedHeaderKey || header.GetValue() != "allow" {
					t.Fatalf("Check() header = %v, want %s:allow", header, allowedHeaderKey)
				}
				return
			}
			if okResp != nil {
				t.Fatalf("Check() returned an OK response for a non-matching payload")
			}
			if deniedResp := resp.GetDeniedResponse(); deniedResp == nil || deniedResp.GetBody() == "" {
				t.Fatalf("Check() denied response = %v, want a body", deniedResp)
			}
		})
	}
}

func jwtMetadata(key, value string) map[string]*structpb.Struct {
	return map[string]*structpb.Struct{
		defaultMetadataNamespace: newStructForTest(map[string]any{
			jwtPayloadKey: map[string]any{key: value},
		}),
	}
}

func claimOutsidePayloadMetadata(key, value string) map[string]*structpb.Struct {
	return map[string]*structpb.Struct{
		defaultMetadataNamespace: newStructForTest(map[string]any{key: value}),
	}
}

func newStructForTest(fields map[string]any) *structpb.Struct {
	value, err := structpb.NewStruct(fields)
	if err != nil {
		panic(err)
	}
	return value
}

func TestHasRequestHeader(t *testing.T) {
	request := &authv3.AttributeContext_HttpRequest{
		Headers: map[string]string{"authorization": "Bearer token", "x-empty": ""},
	}

	if !hasRequestHeader(request, "Authorization") {
		t.Fatal("hasRequestHeader() = false, want case-insensitive match")
	}
	if hasRequestHeader(request, "x-empty") {
		t.Fatal("hasRequestHeader() = true, want false for an empty header value")
	}
	if hasRequestHeader(request, "x-missing") {
		t.Fatal("hasRequestHeader() = true, want false for a missing header")
	}
}

func TestCheckUsesRequiredHeader(t *testing.T) {
	originalSkip, originalHeader := *skipJWTMetadata, *requiredHeader
	defer func() {
		*skipJWTMetadata, *requiredHeader = originalSkip, originalHeader
	}()
	*skipJWTMetadata, *requiredHeader = true, "authorization"

	check := func(headers map[string]string) *authv3.CheckResponse {
		t.Helper()
		resp, err := (&authorizationServer{}).Check(context.Background(), &authv3.CheckRequest{
			Attributes: &authv3.AttributeContext{
				Request: &authv3.AttributeContext_Request{
					Http: &authv3.AttributeContext_HttpRequest{Headers: headers},
				},
			},
		})
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		return resp
	}

	if got := check(map[string]string{"Authorization": "Bearer token"}).GetStatus().GetCode(); got != int32(codes.OK) {
		t.Fatalf("Check() status = %v, want OK with required header", got)
	}
	if got := check(nil).GetStatus().GetCode(); got != int32(codes.PermissionDenied) {
		t.Fatalf("Check() status = %v, want PermissionDenied without required header", got)
	}
}
