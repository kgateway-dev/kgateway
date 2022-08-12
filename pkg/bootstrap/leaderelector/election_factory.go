package leaderelector

import "context"

// ElectionConfig is the set of properties that can be used to configure leader elections
type ElectionConfig struct {
	// The name of the component
	Id string
	// The namespace where the component is running
	Namespace string
	// Callback function that is executed when the current component becomes leader
	OnStartedLeading func(c context.Context)
	// Callback function that is executed when the current component stops leading
	OnStoppedLeading func()
	// Callback function that is executed when a new leader is elected
	OnNewLeader func(leaderId string)
	// ReleaseOnCancel should be set true if the lock should be released
	// when the run context is cancelled. If you set this to true, you must
	// ensure all code guarded by this lease has successfully completed
	// prior to cancelling the context, or you may have two processes
	// simultaneously acting on the critical path.
	ReleaseOnCancel bool
}

// An ElectionFactory is an implementation for running a leader election
type ElectionFactory interface {
	// StartElection begins leader election and returns the Identity of the current component
	StartElection(ctx context.Context, config *ElectionConfig) (Identity, error)
}
