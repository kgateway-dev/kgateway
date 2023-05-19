package serviceconverter

import (
	"context"
	"strconv"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	kubev1 "k8s.io/api/core/v1"
)

/*
The values for these annotations can be provided in one of two ways:

gloo.solo.io/sslService.secret = mysecret

OR

gloo.solo.io/sslService.secret = 443:mysecret

The former will use ssl on all ports for the service
The latter will use ssl only on port 443 of the service

*/

const GlooSslSecretAnnotation = "gloo.solo.io/sslService.secret"
const GlooSslTlsCertAnnotation = "gloo.solo.io/sslService.tlsCert"
const GlooSslTlsKeyAnnotation = "gloo.solo.io/sslService.tlsKey"
const GlooSslRootCaAnnotation = "gloo.solo.io/sslService.rootCa"

// sets UseSsl on the upstream if the service has the relevant port name
type UseSslConverter struct{}

func (u *UseSslConverter) ConvertService(_ context.Context, svc *kubev1.Service, port kubev1.ServicePort, us *v1.Upstream) error {

	upstreamSslConfig := upstreamSslConfigFromService(svc, port)

	if upstreamSslConfig != nil {
		us.SslConfig = upstreamSslConfig
	}

	return nil
}

func upstreamSslConfigFromService(svc *kubev1.Service, svcPort kubev1.ServicePort) *ssl.UpstreamSslConfig {

	if svc.Annotations == nil {
		return nil
	}

	// returns empty string if the target port is specified and it's not equal to the serve port
	getAnnotationValue := func(key string) string {
		valWithPort := svc.Annotations[key]

		val, port := splitPortFromValue(valWithPort)
		if port == 0 || port == svcPort.Port {
			return val
		}
		return ""
	}

	secretName := getAnnotationValue(GlooSslSecretAnnotation)
	tlsCert := getAnnotationValue(GlooSslTlsCertAnnotation)
	tlsKey := getAnnotationValue(GlooSslTlsKeyAnnotation)
	rootCa := getAnnotationValue(GlooSslRootCaAnnotation)

	switch {
	case secretName != "":
		return &ssl.UpstreamSslConfig{
			SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
				SecretRef: &core.ResourceRef{
					Name:      secretName,
					Namespace: svc.Namespace,
				},
			},
		}
	case tlsCert != "" || tlsKey != "" || rootCa != "":
		return &ssl.UpstreamSslConfig{
			SslSecrets: &ssl.UpstreamSslConfig_SslFiles{
				SslFiles: &ssl.SSLFiles{
					TlsCert: tlsCert,
					TlsKey:  tlsKey,
					RootCa:  rootCa,
				},
			},
		}
	}

	return nil
}

func splitPortFromValue(value string) (string, int32) {
	split := strings.Split(value, ":")
	if len(split) != 2 {
		return value, 0
	}
	i, _ := strconv.Atoi(split[0])
	return split[1], int32(i)
}
