package kube

import (
	"context"
	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/go-utils/contextutils"
	"k8s.io/client-go/rest"
	k8sleaderelection "k8s.io/client-go/tools/leaderelection"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/leaderelection"
	"time"
)

var _ leaderelector.ElectionFactory = new(kubeElectionFactory)

type kubeElectionFactory struct {
	restCfg *rest.Config
}

func NewKubeElectionFactory(config *rest.Config) *kubeElectionFactory {
	return &kubeElectionFactory{
		restCfg: config,
	}
}

func (f *kubeElectionFactory) StartElection(ctx context.Context, config leaderelector.ElectionConfig) (leaderelector.Identity, error) {
	var leader = pointer.BoolPtr(false)
	identity := leaderelector.NewIdentity(leader)

	leOpts := leaderelection.Options{
		LeaderElection:          true,
		LeaderElectionID:        config.Id,
		LeaderElectionNamespace: config.Namespace,
	}
	// Create the resource Lock interface necessary for leader election.
	// Controller runtime requires an event handler provider, but that package is
	// internal so for right now we pass a noop handler.
	resourceLock, err := leaderelection.NewResourceLock(f.restCfg, NewNoopProvider(), leOpts)
	if err != nil {
		return identity, err
	}

	l, err := k8sleaderelection.NewLeaderElector(
		k8sleaderelection.LeaderElectionConfig{
			Lock:          resourceLock,
			LeaseDuration: 15 * time.Second, // Default value according to docs
			RenewDeadline: 10 * time.Second, // Default value according to docs
			RetryPeriod:   2 * time.Second,  // Default value according to docs
			Callbacks: k8sleaderelection.LeaderCallbacks{
				OnStartedLeading: func(callbackCtx context.Context) {
					contextutils.LoggerFrom(ctx).Debugf("Started Leading")
					*leader = true
					config.OnStartedLeading(callbackCtx)
				},
				OnStoppedLeading: func() {
					contextutils.LoggerFrom(ctx).Error("Stopped Leading")
					*leader = false
					config.OnStoppedLeading()
				},
				OnNewLeader: func(identity string) {
					contextutils.LoggerFrom(ctx).Debugf("New Leader Elected with Identity: %s", identity)
					config.OnNewLeader(identity)
				},
			},
			Name: config.Id,
		},
	)
	if err != nil {
		return identity, err
	}

	// Start the leader elector process in a goroutine
	contextutils.LoggerFrom(ctx).Debugf("Starting Kube Leader Election")
	go l.Run(ctx)
	return identity, nil
}
