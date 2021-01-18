package register

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/skv2/pkg/multicluster/kubeconfig"
	"github.com/solo-io/skv2/pkg/multicluster/register"
	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Register(opts *options.Options) error {
	ctx := context.Background()
	registerOpts := opts.Cluster.Register
	mgmtKubeCfg, err := kubeconfig.GetClientConfigWithContext("", "", "")
	if err != nil {
		return err
	}
	remoteKubeCfg, err := kubeconfig.GetClientConfigWithContext(registerOpts.RemoteKubeConfig, registerOpts.RemoteContext, "")
	if err != nil {
		return err
	}

	clusterRegisterOpts := register.RegistrationOptions{
		KubeCfg:          mgmtKubeCfg,
		RemoteKubeCfg:    remoteKubeCfg,
		RemoteCtx:        registerOpts.RemoteContext,
		APIServerAddress: registerOpts.LocalClusterDomainOverride,
		ClusterName:      registerOpts.ClusterName,
		Namespace:        opts.Cluster.FederationNamespace,
		RemoteNamespace:  registerOpts.RemoteNamespace,
		ClusterRoles: []*v1.ClusterRole{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: registerOpts.RemoteNamespace,
					Name:      "gloo-federation-controller",
				},
				Rules: glooFederationPolicyRules,
			},
		},
		Roles: []*v1.Role{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: registerOpts.RemoteNamespace,
					Name:      "gloo-federation-controller",
				},
				Rules: glooFederationReadConfigPolicyRules,
			},
		},
	}

	return clusterRegisterOpts.RegisterCluster(ctx)
}
