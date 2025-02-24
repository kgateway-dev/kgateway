package pluginutils

import (
	"fmt"

	"github.com/rotisserie/eris"
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
)

func GetSecretIr(secrets *krtcollections.SecretIndex, krtctx krt.HandlerContext, secretName, ns string) (*ir.Secret, error) {
	secretRef := gwv1.SecretObjectReference{
		Name: gwv1.ObjectName(secretName),
	}
	secret, err := secrets.GetSecret(krtctx, krtcollections.From{GroupKind: v1alpha1.UpstreamGVK.GroupKind(), Namespace: ns}, secretRef)
	if secret != nil {
		return secret, nil
	} else {
		return nil, eris.Wrapf(err, fmt.Sprintf("unable to find the secret %s", secretRef.Name))
	}
}
