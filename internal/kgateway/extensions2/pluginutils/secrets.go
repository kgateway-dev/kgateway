package pluginutils

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func GetSecretIr(secrets *krtcollections.SecretIndex, krtctx krt.HandlerContext, secretName, ns string) (*ir.Secret, error) {
	secretRef := gwv1.SecretObjectReference{
		Name: gwv1.ObjectName(secretName),
	}
	secret, err := secrets.GetSecret(krtctx, krtcollections.From{GroupKind: wellknown.BackendGVK.GroupKind(), Namespace: ns}, secretRef)
	if err != nil {
		return nil, fmt.Errorf("failed to find secret %s: %v", secretName, err)
	}
	return secret, nil
}
