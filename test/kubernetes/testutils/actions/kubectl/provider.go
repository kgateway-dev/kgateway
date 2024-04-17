package kubectl

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"
)

// Provider provides a mechanism to generation operations that are performed via kubectl
type Provider struct {
	kubeCli *kubectl.Cli
}

func NewProvider() *Provider {
	return &Provider{
		kubeCli: nil,
	}
}

// WithClusterCli sets the Provider to use a Cli
func (p *Provider) WithClusterCli(kubeCli *kubectl.Cli) *Provider {
	p.kubeCli = kubeCli
	return p
}

// Client returns the kubectl.Cli
func (p *Provider) Client() *kubectl.Cli {
	return p.kubeCli
}

func (p *Provider) NewApplyManifestAction(manifest string, args ...string) actions.ClusterAction {
	return func(ctx context.Context) error {
		return p.kubeCli.ApplyFile(ctx, manifest, args...)
	}
}

func (p *Provider) NewDeleteManifestAction(manifest string, args ...string) actions.ClusterAction {
	return func(ctx context.Context) error {
		return p.kubeCli.DeleteFile(ctx, manifest, args...)
	}
}
