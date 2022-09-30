package leaderelector

var _ Identity = new(identityImpl)

// Identity contains leader election information about the current component
type Identity interface {
	// IsLeader returns true if the current component is the leader, false otherwise
	IsLeader() bool

	// Elected returns the channel that will be signaled when the current component is elected the leader
	Elected() <-chan struct{}
}

type identityImpl struct {
	elected <-chan struct{}
}

func NewIdentity(elected <-chan struct{}) *identityImpl {
	return &identityImpl{
		elected: elected,
	}
}

func (i identityImpl) IsLeader() bool {
	channelOpen := true
	select {
	case _, channelOpen = <-i.Elected():
	default:
		// https://go.dev/tour/concurrency/6
		// Ensure that receiving from elected channel does not block
	}

	// Leadership is designated by the closing of the election channel
	return !channelOpen
}

func (i identityImpl) Elected() <-chan struct{} {
	return i.elected
}
