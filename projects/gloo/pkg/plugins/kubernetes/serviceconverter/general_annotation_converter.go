package serviceconverter

import (
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/utils/protoutils"
	kubev1 "k8s.io/api/core/v1"
)

func init() {
	DefaultServiceConverters = append(DefaultServiceConverters, &GeneralServiceConverter{})
}

const GlooAnnotationPrefix = "gloo.solo.io/UpstreamConfig"

type GeneralServiceConverter struct{}

func (s *GeneralServiceConverter) ConvertService(svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error {
	upstreamConfigJson, ok := svc.Annotations[GlooAnnotationPrefix]
	if !ok {
		return nil
	}

	var spec v1.Upstream
	if err := protoutils.UnmarshalResource([]byte(upstreamConfigJson), &spec); err != nil {
		return err
	}

	translator.MergeUpstreams(us, &spec)

	return nil
}
