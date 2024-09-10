package secrets

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewSecretsController(ctx context.Context, mgr manager.Manager, inputChannels *proxy_syncer.GatewayInputChannels) error {
	cb := &controllerBuilder{
		mgr: mgr,
		reconciler: &controllerReconciler{
			cli:           mgr.GetClient(),
			inputChannels: inputChannels,
		},
	}
	return run(ctx, cb.watchSecrets)
}

func run(ctx context.Context, funcs ...func(ctx context.Context) error) error {
	for _, f := range funcs {
		if err := f(ctx); err != nil {
			return err
		}
	}
	return nil
}

type controllerBuilder struct {
	mgr        manager.Manager
	reconciler *controllerReconciler
}

func (c *controllerBuilder) watchSecrets(ctx context.Context) error {
	err := ctrl.NewControllerManagedBy(c.mgr).
		For(&corev1.Secret{}).
		Complete(reconcile.Func(c.reconciler.ReconcileSecrets))
	if err != nil {
		return err
	}
	return nil
}

type controllerReconciler struct {
	cli           client.Client
	inputChannels *proxy_syncer.GatewayInputChannels
}

func (r *controllerReconciler) ReconcileSecrets(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	secretList := &corev1.SecretList{}
	if err := r.cli.List(ctx, secretList); err != nil {
		return ctrl.Result{}, err
	}

	skSecretList := v1.SecretList{}
	for _, secret := range secretList.Items {
		secret := secret
		// Only try and parse TLS/Opaque secrets for now
		if secret.Type != corev1.SecretTypeTLS && secret.Type != corev1.SecretTypeOpaque {
			continue
		}
		skSecretList = append(skSecretList, &v1.Secret{
			Kind: &v1.Secret_Tls{
				Tls: &v1.TlsSecret{
					PrivateKey: string(secret.Data[corev1.TLSPrivateKeyKey]),
					CertChain:  string(secret.Data[corev1.TLSCertKey]),
					RootCa:     string(secret.Data[corev1.ServiceAccountRootCAKey]),
				},
			},
			Metadata: kubeutils.FromKubeMeta(secret.ObjectMeta, true),
		})

	}
	r.inputChannels.UpdateSecretInputs(ctx, proxy_syncer.SecretInputs{
		Secrets: skSecretList,
	})
	return ctrl.Result{}, nil
}
