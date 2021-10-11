package serviceconverter

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubev1 "k8s.io/api/core/v1"
)

// ServiceConverters apply extra changes to an upstream spec before the upstream is created
// use this to support things like custom config from annotations
type ServiceConverter interface {
	ConvertService(svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error
}

// the default annotation converters that will be used
// these are initialized at runtime
var DefaultServiceConverters []ServiceConverter
