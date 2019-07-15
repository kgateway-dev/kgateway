package glooec2

import "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

// UpstreamSpecRef is a in-memory helper for passing UpstreamSpec data around after an Upstream has been resolved to
// an AWS EC2 upstream. This way, you don't need to pass a ref and a spec to functions that need both
type UpstreamSpecRef struct {
	Spec *UpstreamSpec
	Ref  core.ResourceRef
}
