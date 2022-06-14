package serviceconverter

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubev1 "k8s.io/api/core/v1"
)

func init() {
	DefaultServiceConverters = []ServiceConverter{
		&UseHttp2Converter{},
		&UseSslConverter{},
		// The General Service Converter is applied last, and is capable of overriding settings applied by prior converters
		&GeneralServiceConverter{},
	}
}

// ServiceConverters apply extra changes to an upstream spec before the upstream is created
// use this to support things like custom config from annotations
type ServiceConverter interface {
	ConvertService(ctx context.Context, svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error
}

// the default annotation converters that will be used
// these are initialized at runtime
var DefaultServiceConverters []ServiceConverter
