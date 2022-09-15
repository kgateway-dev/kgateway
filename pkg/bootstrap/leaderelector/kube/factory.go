package kube

import (
	"context"
	"os"
	"time"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/atomic"
	"k8s.io/client-go/rest"
	k8sleaderelection "k8s.io/client-go/tools/leaderelection"
	"sigs.k8s.io/controller-runtime/pkg/leaderelection"
)

var _ leaderelector.ElectionFactory = new(kubeElectionFactory)

// kubeElectionFactory is the implementation for coordinating leader election using
// the k8s leader election tool: https://github.com/kubernetes/client-go/tree/master/tools/leaderelection
type kubeElectionFactory struct {
	restCfg *rest.Config
}

func NewElectionFactory(config *rest.Config) *kubeElectionFactory {
	return &kubeElectionFactory{
		restCfg: config,
	}
}

func (f *kubeElectionFactory) StartElection(ctx context.Context, config *leaderelector.ElectionConfig) (leaderelector.Identity, error) {
	var (
		elected = make(chan struct{})
		leader  = atomic.NewBool(false)
	)

	identity := leaderelector.NewIdentity(leader, elected)

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
			Lock: resourceLock,
			// Define the following values according to the defaults:
			// https://github.com/kubernetes/client-go/blob/master/tools/leaderelection/leaderelection.go
			LeaseDuration: getLeaseDuration(),
			RenewDeadline: 10 * time.Second,
			RetryPeriod:   2 * time.Second,
			Callbacks: k8sleaderelection.LeaderCallbacks{
				OnStartedLeading: func(callbackCtx context.Context) {
					contextutils.LoggerFrom(callbackCtx).Debug("Started Leading")
					leader.Store(true)
					elected <- struct{}{}
					config.OnStartedLeading(callbackCtx)
				},
				OnStoppedLeading: func() {
					contextutils.LoggerFrom(ctx).Error("Stopped Leading")
					leader.Store(false)
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

	// Wait for the context to be cancelled to close the elected channel
	go func(ctx context.Context) {
		defer close(elected)
		<-ctx.Done()
	}(ctx)

	// Start the leader elector process in a goroutine
	contextutils.LoggerFrom(ctx).Debug("Starting Kube Leader Election")
	go l.Run(ctx)

	return identity, nil
}

func getLeaseDuration() time.Duration {
	// https://github.com/kubernetes/client-go/blob/master/tools/leaderelection/leaderelection.go
	leaseDuration := 15 * time.Second

	leaseDurationStr := os.Getenv("LEADER_ELECTION_LEASE_DURATION")
	if leaseDurationStr != "" {
		if dur, err := time.ParseDuration(leaseDurationStr); err != nil {
			leaseDuration = dur
		}
	}

	return leaseDuration
}
