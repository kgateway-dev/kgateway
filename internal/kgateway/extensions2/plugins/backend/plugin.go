package backend

import (
	"bytes"
	"context"
	"fmt"
	"maps"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/solo-io/go-utils/contextutils"
	"istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

const (
	ExtensionName = "backend"
)

// BackendIr is the internal representation of a backend.
type BackendIr struct {
	AwsSecret *ir.Secret
}

func (u *BackendIr) data() map[string][]byte {
	if u.AwsSecret == nil {
		return nil
	}
	return u.AwsSecret.Data
}

// TODO: Is this missing CreationTime? The interface check is failing.

func (u *BackendIr) Equals(other any) bool {
	otherUpstream, ok := other.(*BackendIr)
	if !ok {
		return false
	}
	return maps.EqualFunc(u.data(), otherUpstream.data(), func(a, b []byte) bool {
		return bytes.Equal(a, b)
	})
}

type backendPlugin struct {
	ir.UnimplementedProxyTranslationPass
	needFilter map[string]bool
}

func registerTypes(ourCli versioned.Interface) {
	kubeclient.Register[*v1alpha1.Backend](
		v1alpha1.BackendGVK.GroupVersion().WithResource("backends"),
		v1alpha1.BackendGVK,
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (runtime.Object, error) {
			return ourCli.GatewayV1alpha1().Backends(namespace).List(context.Background(), o)
		},
		func(c kubeclient.ClientGetter, namespace string, o metav1.ListOptions) (watch.Interface, error) {
			return ourCli.GatewayV1alpha1().Backends(namespace).Watch(context.Background(), o)
		},
	)
}

func NewPlugin(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin {
	registerTypes(commoncol.OurClient)

	col := krt.WrapClient(kclient.New[*v1alpha1.Backend](commoncol.Client), commoncol.KrtOpts.ToOptions("Backends")...)

	gk := v1alpha1.BackendGVK.GroupKind()
	translateFn := buildTranslateFunc(commoncol.Secrets)

	bcol := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.BackendObjectIR {
		objIR, err := translateFn(krtctx, i)
		if err != nil {
			contextutils.LoggerFrom(ctx).Errorf("failed to translate backend %s/%s: %v", i.GetNamespace(), i.GetName(), err)
			return nil
		}

		return &ir.BackendObjectIR{
			ObjectSource: ir.ObjectSource{
				Kind:      gk.Kind,
				Group:     gk.Group,
				Namespace: i.GetNamespace(),
				Name:      i.GetName(),
			},
			GvPrefix:          ExtensionName,
			CanonicalHostname: hostname(i),
			Obj:               i,
			ObjIr:             objIR,
		}
	})
	endpoints := krt.NewCollection(col, func(krtctx krt.HandlerContext, i *v1alpha1.Backend) *ir.EndpointsForBackend {
		return processEndpoints(i)
	})
	return extensionsplug.Plugin{
		ContributesBackends: map[schema.GroupKind]extensionsplug.BackendPlugin{
			gk: {
				BackendInit: ir.BackendInit{
					InitBackend: processUpstream,
				},
				Endpoints: endpoints,
				Backends:  bcol,
			},
		},
	}
}

func buildTranslateFunc(secrets *krtcollections.SecretIndex) func(krtctx krt.HandlerContext, i *v1alpha1.Backend) (*BackendIr, error) {
	return func(krtctx krt.HandlerContext, i *v1alpha1.Backend) (*BackendIr, error) {
		if i.Spec.Type != v1alpha1.BackendTypeAWS {
			// we only need to build an IR for AWS backends.
			return nil, nil
		}
		if i.Spec.Aws.Auth == nil {
			// if auth is not specified, we use instance metadata.
			return nil, nil
		}
		var ir BackendIr
		if i.Spec.Aws.Auth.Secret != nil {
			ns := i.GetNamespace()
			secretRef := gwv1.SecretObjectReference{
				Name: gwv1.ObjectName(i.Spec.Aws.Auth.Secret.Name),
			}
			secret, err := secrets.GetSecret(krtctx, krtcollections.From{GroupKind: v1alpha1.BackendGVK.GroupKind(), Namespace: ns}, secretRef)
			if err != nil {
				return nil, fmt.Errorf("failed to get secret: %v", err)
			}
			if secret != nil {
				ir.AwsSecret = secret
			}
		}
		return &ir, nil
	}
}

func processUpstream(ctx context.Context, in ir.BackendObjectIR, out *envoy_config_cluster_v3.Cluster) {
	log := contextutils.LoggerFrom(ctx)

	up, ok := in.Obj.(*v1alpha1.Backend)
	if !ok {
		log.Error("failed to cast backend object")
		return
	}
	ir, ok := in.ObjIr.(*BackendIr)
	if !ok {
		log.Error("failed to cast backend ir")
		return
	}

	spec := up.Spec
	switch {
	case spec.Type == v1alpha1.BackendTypeStatic:
		if err := processStatic(ctx, spec.Static, out); err != nil {
			log.Error("failed to process static backend", "error", err)
		}
	case spec.Type == v1alpha1.BackendTypeAWS:
		if err := processAws(ctx, spec.Aws, ir, out); err != nil {
			log.Error("failed to process aws backend", "error", err)
		}
	}
}

// hostname returns the hostname for the backend. Only static backends are supported.
func hostname(in *v1alpha1.Backend) string {
	if in.Spec.Type != v1alpha1.BackendTypeStatic {
		return ""
	}
	if len(in.Spec.Static.Hosts) == 0 {
		return ""
	}
	return string(in.Spec.Static.Hosts[0].Host)
}

// processEndpoints returns the endpoints for the backend. Only static backends are supported.
func processEndpoints(in *v1alpha1.Backend) *ir.EndpointsForBackend {
	if in.Spec.Type != v1alpha1.BackendTypeStatic {
		return nil
	}
	return processEndpointsStatic(in.Spec.Static)
}
