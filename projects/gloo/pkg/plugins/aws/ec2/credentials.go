package ec2

import (
	"fmt"

	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/ec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// a credential spec represents an AWS client's view into AWS credentialMap
// we expect multiple upstreams to share the same view (so we batch the queries and apply filters locally)
type CredentialSpec struct {
	// secretRef identifies the AWS secret that should be used to authenticate the client
	secretRef *core.ResourceRef
	// region is the AWS region where our credentialMap live
	region string
	// roleArn is an AWS Roles (specified by its Amazon Resource Number (ARN)) which should be assumed when
	// querying for instances available to the upstream
	roleArn string
}

// https://docs.aws.amazon.com/general/latest/gr/aws-arns-and-namespaces.html#arn-syntax-ec2
const arnSegmentDelimiter = ":"

func (cs *CredentialSpec) GetKey() CredentialKey {
	// use a very conservative "hash" strategy to avoid having to depend on aws's arn specification
	return CredentialKey(fmt.Sprintf("%v-%v-%v", cs.secretRef.String(), cs.region, cs.roleArn))
}

func (cs *CredentialSpec) Region() string {
	return cs.region
}

func (cs *CredentialSpec) SecretRef() *core.ResourceRef {
	return cs.secretRef
}

func (cs *CredentialSpec) Arn() string {
	return cs.roleArn
}

func (cs *CredentialSpec) Clone() *CredentialSpec {
	return &CredentialSpec{
		secretRef: cs.secretRef,
		region:    cs.region,
		roleArn:   cs.roleArn,
	}
}

func NewCredentialSpecFromEc2UpstreamSpec(spec *glooec2.UpstreamSpec) *CredentialSpec {
	return &CredentialSpec{
		secretRef: spec.SecretRef,
		region:    spec.Region,
		roleArn:   spec.GetRoleArn(),
	}
}

// Since "==" is not defined for slices, slices (in particular, the roleArns slice) cannot be used as keys for go maps.
// Instead, we will use a string form. We give it a name for clarity.
type CredentialKey string
