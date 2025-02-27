package common

import (
	"github.com/go-logr/logr"
	"istio.io/istio/pkg/kube"
	istiokube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/client/clientset/versioned"
)

type CommonCollections struct {
	OurClient versioned.Interface
	Client    kube.Client
	KrtOpts   krtutil.KrtOptions
	Secrets   *krtcollections.SecretIndex
	Backends  *krtcollections.BackendIndex

	Pods      krt.Collection[krtcollections.LocalityPod]
	RefGrants *krtcollections.RefGrantIndex

	// static set of global Settings, non-krt based for dev speed
	// TODO: this should be refactored to a more correct location,
	// or even better, be removed entirely and done per Gateway (maybe in GwParams)
	Settings settings.Settings
}

func (c *CommonCollections) HasSynced() bool {
	return c.Secrets.HasSynced() && c.Pods.HasSynced() && c.RefGrants.HasSynced()
}

func NewCommonCollections(
	krtOptions krtutil.KrtOptions,
	client istiokube.Client,
	ourClient versioned.Interface,
	logger logr.Logger,
	settings settings.Settings,
) *CommonCollections {
	secretClient := kclient.New[*corev1.Secret](client)
	k8sSecretsRaw := krt.WrapClient(secretClient, krt.WithStop(krtOptions.Stop), krt.WithName("Secrets") /* no debug here - we don't want raw secrets printed*/)
	k8sSecrets := krt.NewCollection(k8sSecretsRaw, func(kctx krt.HandlerContext, i *corev1.Secret) *ir.Secret {
		res := ir.Secret{
			ObjectSource: ir.ObjectSource{
				Group:     "",
				Kind:      "Secret",
				Namespace: i.Namespace,
				Name:      i.Name,
			},
			Obj:  i,
			Data: i.Data,
		}
		return &res
	}, krtOptions.ToOptions("secrets")...)
	secrets := map[schema.GroupKind]krt.Collection[ir.Secret]{
		{Group: "", Kind: "Secret"}: k8sSecrets,
	}

	refgrantsCol := krt.WrapClient(kclient.New[*gwv1beta1.ReferenceGrant](client), krtOptions.ToOptions("RefGrants")...)
	refgrants := krtcollections.NewRefGrantIndex(refgrantsCol)

	return &CommonCollections{
		OurClient: ourClient,
		Client:    client,
		KrtOpts:   krtOptions,
		Secrets:   krtcollections.NewSecretIndex(secrets, refgrants),
		Pods:      krtcollections.NewPodsCollection(client, krtOptions),
		RefGrants: refgrants,
		Settings:  settings,
	}
}
