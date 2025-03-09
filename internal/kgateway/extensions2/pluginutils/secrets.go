package pluginutils

import (
	"fmt"

	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// TODO: move to internal/kgateway/krtcollections package near the secret index collection?
func GetSecretIr(secrets *krtcollections.SecretIndex, krtctx krt.HandlerContext, secretName, ns string) (*ir.Secret, error) {
	secretRef := gwv1.SecretObjectReference{
		Name: gwv1.ObjectName(secretName),
	}
	from := krtcollections.From{
		GroupKind: wellknown.BackendGVK.GroupKind(),
		Namespace: ns,
	}
	secret, err := secrets.GetSecret(krtctx, from, secretRef)
	if secret != nil {
		return secret, nil
	} else {
		return nil, fmt.Errorf(fmt.Sprintf("unable to find the secret %s", secretRef.Name), err)
	}
}
