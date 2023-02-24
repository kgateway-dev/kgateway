package aws_credentials

import (
	"context"

	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"

	ec2api "github.com/aws/aws-sdk-go/service/ec2"

	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws/ec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("", func() {
	if testutils.ShouldSkipTempDisabledTests() {
		return
	}
	var (
		ctx    context.Context
		cancel context.CancelFunc

		secret *v1.Secret
	)

	createSecret := func() {
		secretClient, err := getInMemorySecretClient(ctx)
		Expect(err).NotTo(HaveOccurred())

		// In the BeforeSuite, we validate that credentials are present
		// Therefore, if somehow we have reached here, we error loudly
		localAwsCredentials := credentials.NewSharedCredentials("", "")
		v, err := localAwsCredentials.Get()
		Expect(err).NotTo(HaveOccurred())

		secret = &v1.Secret{
			Metadata: &core.Metadata{
				Namespace: "default",
				Name:      "secret",
			},
			Kind: &v1.Secret_Aws{
				Aws: &v1.AwsSecret{
					AccessKey: v.AccessKeyID,
					SecretKey: v.SecretAccessKey,
				},
			},
		}
		_, err = secretClient.Write(secret, clients.WriteOpts{Ctx: ctx})
		Expect(err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		createSecret()
	})

	AfterEach(func() {
		// The secret we created is stored in memory, so it will be cleaned up between test runs
		cancel()
	})

	It("should assume role correctly", func() {
		var filters []*glooec2.TagFilter
		roleArn := "arn:aws:iam::802411188784:role/gloo-edge-e2e-sts" // this role will not have correct perms for now - tests are currently disabled
		withRole := &v1.Upstream{
			UpstreamType: &v1.Upstream_AwsEc2{
				AwsEc2: &glooec2.UpstreamSpec{
					Region:    region,
					SecretRef: secret.Metadata.Ref(),
					//todo - figure out what credentials this role needs for this test
					RoleArn:  roleArn,
					Filters:  filters,
					PublicIp: false,
					Port:     80,
				},
			},
			Metadata: &core.Metadata{Name: "with-role", Namespace: "default"},
		}
		withRoleWithoutSecret := &v1.Upstream{
			UpstreamType: &v1.Upstream_AwsEc2{
				AwsEc2: &glooec2.UpstreamSpec{
					Region: region,
					//todo - figure out what credentials this role needs for this test
					RoleArn:  roleArn,
					Filters:  filters,
					PublicIp: false,
					Port:     80,
				},
			},
			Metadata: &core.Metadata{Name: "with-role-without-secret", Namespace: "default"},
		}
		withOutRole := &v1.Upstream{
			UpstreamType: &v1.Upstream_AwsEc2{
				AwsEc2: &glooec2.UpstreamSpec{
					Region:    region,
					SecretRef: secret.Metadata.Ref(),
					Filters:   filters,
					PublicIp:  false,
					Port:      80,
				},
			},
			Metadata: &core.Metadata{Name: "without-role", Namespace: "default"},
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

func getInMemorySecretClient(ctx context.Context) (v1.SecretClient, error) {
	secretClientFactory := &factory.MemoryResourceClientFactory{
		Cache: memory.NewInMemoryResourceCache(),
	}
	secretClient, err := v1.NewSecretClient(ctx, secretClientFactory)
	if err != nil {
		return nil, eris.Wrapf(err, "creating Secrets client")
	}
	if err = secretClient.Register(); err != nil {
		return nil, err
	}
	return secretClient, nil
}
