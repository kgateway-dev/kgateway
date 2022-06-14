package serviceconverter

import (
	"context"
	"strings"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"

	"github.com/golang/protobuf/ptypes/wrappers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubev1 "k8s.io/api/core/v1"
)

const GlooH2Annotation = "gloo.solo.io/h2_service"

var http2PortNames = []string{
	"grpc",
	"h2",
	"http2",
}

// UseHttp2Converter sets UseHttp2 on the upstream if:
// (1) the service has the "h2_service" annotation; or
// (2) the "h2_service" annotation defined in Settings.UpstreamOptions; or
// (3) the service has the relevant port name
type UseHttp2Converter struct{}

func (u *UseHttp2Converter) ConvertService(ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error {
	us.UseHttp2 = useHttp2(ctx, svc, port)
	return nil
}

func useHttp2(ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort) *wrappers.BoolValue {
	if svc.Annotations != nil {
		if svc.Annotations[GlooH2Annotation] == "true" {
			return &wrappers.BoolValue{Value: true}
		} else if svc.Annotations[GlooH2Annotation] == "false" {
			return &wrappers.BoolValue{Value: false}
		}
	}
	if globalAnnotations := settingsutil.MaybeFromContext(ctx).GetUpstreamOptions().GetGlobalAnnotations(); globalAnnotations != nil {
		if globalAnnotations[GlooH2Annotation] == "true" {
			return &wrappers.BoolValue{Value: true}
		} else if globalAnnotations[GlooH2Annotation] == "false" {
			return &wrappers.BoolValue{Value: false}
		}
	}

	for _, http2Name := range http2PortNames {
		if strings.HasPrefix(port.Name, http2Name) {
			return &wrappers.BoolValue{Value: true}
		}
	}

	return nil
}
