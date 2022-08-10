package leaderelector

var _ Identity = new(IdentityImpl)

type Identity interface {
	IsLeader() bool
}

type IdentityImpl struct {
	leader bool
}

func NewIdentity(leader *bool) *IdentityImpl {
	return &IdentityImpl{
		leader: *leader,
	}
}

func (i IdentityImpl) IsLeader() bool {
	return i.leader
}
