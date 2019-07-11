package awscache

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// a credential spec represents an AWS client's view into AWS credentialMap
// we expect multiple upstreams to share the same view (so we batch the queries and apply filters locally)
type credentialSpec struct {
	// secretRef identifies the AWS secret that should be used to authenticate the client
	secretRef core.ResourceRef
	// region is the AWS region where our credentialMap live
	region string
}

func credentialSpecFromUpstreamSpec(ec2Spec *glooec2.UpstreamSpec) credentialSpec {
	return credentialSpec{
		secretRef: ec2Spec.SecretRef,
		region:    ec2Spec.Region,
	}
}
