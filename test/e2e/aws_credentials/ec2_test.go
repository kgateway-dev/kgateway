package aws_credentials

import (
	"os"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"

	ec2api "github.com/aws/aws-sdk-go/service/ec2"

	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws/ec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo"

	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("", func() {
	const region = "us-east-1"

	var (
		secret       *v1.Secret
		secretClient v1.SecretClient
		roleArn      string
	)

	addCredentials := func() {
		localAwsCredentials := credentials.NewSharedCredentials("", "")
		v, err := localAwsCredentials.Get()
		if err != nil {
			Skip("no AWS creds available")
		}
		// role arn format: "arn:aws:iam::[account_number]:role/[role_name]"
		roleArn = os.Getenv("AWS_ARN_ROLE_1")
		if roleArn == "" {
			Skip("no AWS role ARN available")
		}
		var opts clients.WriteOpts

		accessKey := v.AccessKeyID
		secretKey := v.SecretAccessKey

		secret = &v1.Secret{
			Metadata: core.Metadata{
				Namespace: "default",
				Name:      region,
			},
			Kind: &v1.Secret_Aws{
				Aws: &v1.AwsSecret{
					AccessKey: accessKey,
					SecretKey: secretKey,
				},
			},
		}

		_, err = secretClient.Write(secret, opts)
		Expect(err).NotTo(HaveOccurred())

	}

	BeforeEach(func() {
		var err error
		secretClient, err = getSecretClient()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Setenv("AWS_ARN_ROLE_1", "arn:aws:iam::410461945957:role/describe-all-ec2-poc")).NotTo(HaveOccurred())

		addCredentials()
	})

	It("should assume role correctly", func() {
		region := "us-east-1"
		secretRef := secret.Metadata.Ref()
		var filters []*glooec2.TagFilter
		withRole := &v1.Upstream{
			UpstreamType: &v1.Upstream_AwsEc2{
				AwsEc2: &glooec2.UpstreamSpec{
					Region:    region,
					SecretRef: &secretRef,
					RoleArn:   roleArn,
					Filters:   filters,
					PublicIp:  false,
					Port:      80,
				},
			},
			Metadata: core.Metadata{Name: "with-role", Namespace: "default"},
		}
		withRoleWithoutSecret := &v1.Upstream{
			UpstreamType: &v1.Upstream_AwsEc2{
				AwsEc2: &glooec2.UpstreamSpec{
					Region:   region,
					RoleArn:  roleArn,
					Filters:  filters,
					PublicIp: false,
					Port:     80,
				},
			},
			Metadata: core.Metadata{Name: "with-role", Namespace: "default"},
		}
		withOutRole := &v1.Upstream{
			UpstreamType: &v1.Upstream_AwsEc2{
				AwsEc2: &glooec2.UpstreamSpec{
					Region:    region,
					SecretRef: &secretRef,
					Filters:   filters,
					PublicIp:  false,
					Port:      80,
				},
			},
			Metadata: core.Metadata{Name: "without-role", Namespace: "default"},
		}

		By("should error when no role provided")
		svcWithout, err := ec2.GetEc2Client(ec2.NewCredentialSpecFromEc2UpstreamSpec(withOutRole.GetAwsEc2()), v1.SecretList{secret})
		Expect(err).NotTo(HaveOccurred())
		_, err = svcWithout.DescribeInstances(&ec2api.DescribeInstancesInput{})
		Expect(err).To(HaveOccurred())

		By("should succeed when role provided, secret passed with upstream")
		svc, err := ec2.GetEc2Client(ec2.NewCredentialSpecFromEc2UpstreamSpec(withRole.GetAwsEc2()), v1.SecretList{secret})
		Expect(err).NotTo(HaveOccurred())
		result, err := svc.DescribeInstances(&ec2api.DescribeInstancesInput{})
		Expect(err).NotTo(HaveOccurred())
		instances := ec2.GetInstancesFromDescription(result)
		Expect(len(instances)).To(BeNumerically(">", 0))

		By("should succeed when role provided, secret derived from env")
		svc, err = ec2.GetEc2Client(ec2.NewCredentialSpecFromEc2UpstreamSpec(withRoleWithoutSecret.GetAwsEc2()), v1.SecretList{secret})
		Expect(err).NotTo(HaveOccurred())
		result, err = svc.DescribeInstances(&ec2api.DescribeInstancesInput{})
		Expect(err).NotTo(HaveOccurred())
		instances = ec2.GetInstancesFromDescription(result)
		Expect(len(instances)).To(BeNumerically(">", 0))
	})

})

func getSecretClient() (v1.SecretClient, error) {
	secretClientFactory := &factory.MemoryResourceClientFactory{
		Cache: memory.NewInMemoryResourceCache(),
	}
	secretClient, err := v1.NewSecretClient(secretClientFactory)
	if err != nil {
		return nil, eris.Wrapf(err, "creating Secrets client")
	}
	if err := secretClient.Register(); err != nil {
		return nil, err
	}
	return secretClient, nil
}
