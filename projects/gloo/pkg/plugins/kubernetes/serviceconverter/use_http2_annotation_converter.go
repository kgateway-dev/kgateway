package serviceconverter

import (
	"strings"

	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubev1 "k8s.io/api/core/v1"
)

func init() {
	DefaultServiceConverters = append(DefaultServiceConverters, &UseHttp2Converter{})
}

const GlooH2Annotation = "gloo.solo.io/h2_service"

var http2PortNames = []string{
	"grpc",
	"h2",
	"http2",
}

// sets UseHttp2 on the upstream if the service has the relevant port name
type UseHttp2Converter struct{}

func (u *UseHttp2Converter) ConvertService(svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error {
	us.UseHttp2 = useHttp2(svc, port)
	return nil
}

func useHttp2(svc *kubev1.Service, port kubev1.ServicePort) *types.BoolValue {
	if svc.Annotations != nil {
		if svc.Annotations[GlooH2Annotation] == "true" {
			return &types.BoolValue{Value: true}
		} else if svc.Annotations[GlooH2Annotation] == "false" {
			return &types.BoolValue{Value: false}
		}
	}

	for _, http2Name := range http2PortNames {
		if strings.HasPrefix(port.Name, http2Name) {
			return &types.BoolValue{Value: true}
		}
	}

	return nil
}
