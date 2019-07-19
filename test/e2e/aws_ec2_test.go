package e2e_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	ec2api "github.com/aws/aws-sdk-go/service/ec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"

	"github.com/solo-io/gloo/pkg/utils"

	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/ec2"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/aws/aws-sdk-go/aws/credentials"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("AWS EC2 Plugin utils test", func() {

	const region = "us-east-1"

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		secret        *gloov1.Secret
		upstream      *gloov1.Upstream
		roleArn       string
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

		secret = &gloov1.Secret{
			Metadata: core.Metadata{
				Namespace: "default",
				Name:      region,
			},
			Kind: &gloov1.Secret_Aws{
				Aws: &gloov1.AwsSecret{
					AccessKey: accessKey,
					SecretKey: secretKey,
				},
			},
		}

		_, err = testClients.SecretClient.Write(secret, opts)
		Expect(err).NotTo(HaveOccurred())

	}
	addUpstream := func() {
		secretRef := secret.Metadata.Ref()
		upstream = &gloov1.Upstream{
			Metadata: core.Metadata{
				Namespace: "default",
				Name:      region,
			},
			UpstreamSpec: &gloov1.UpstreamSpec{
				UpstreamType: &gloov1.UpstreamSpec_AwsEc2{
					AwsEc2: &glooec2.UpstreamSpec{
						Region:    region,
						SecretRef: &secretRef,
						RoleArns:  nil,
						Filters:   nil,
						PublicIp:  true,
						Port:      80,
					},
				},
			},
		}

		var opts clients.WriteOpts
		_, err := testClients.UpstreamClient.Write(upstream, opts)
		Expect(err).NotTo(HaveOccurred())

	}

	validateEc2Endpoint := func(envoyPort uint32, substring string) {

		Eventually(func() (string, error) {

			res, err := http.Get(fmt.Sprintf("http://%s:%d/", "localhost", envoyPort))
			if err != nil {
				return "", errors.Wrapf(err, "unable to call GET")
			}
			if res.StatusCode != http.StatusOK {
				return "", errors.New(fmt.Sprintf("%v is not OK", res.StatusCode))
			}

			defer res.Body.Close()
			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				return "", errors.Wrapf(err, "unable to read body")
			}

			return string(body), nil
		}, "10s", "1s").Should(ContainSubstring(substring))
	}
	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})
	It("should assume role correctly", func() {
		region := "us-east-1"
		secretRef := secret.Metadata.Ref()
		var filters []*glooec2.TagFilter
		withRole := &gloov1.Upstream{
			UpstreamSpec: &gloov1.UpstreamSpec{
				UpstreamType: &gloov1.UpstreamSpec_AwsEc2{
					AwsEc2: &glooec2.UpstreamSpec{
						Region:    region,
						SecretRef: &secretRef,
						RoleArns:  []string{roleArn},
						Filters:   filters,
						PublicIp:  false,
						Port:      80,
					},
				},
			},
			Metadata: core.Metadata{Name: "with-role", Namespace: "default"},
		}
		withOutRole := &gloov1.Upstream{
			UpstreamSpec: &gloov1.UpstreamSpec{
				UpstreamType: &gloov1.UpstreamSpec_AwsEc2{
					AwsEc2: &glooec2.UpstreamSpec{
						Region:    region,
						SecretRef: &secretRef,
						Filters:   filters,
						PublicIp:  false,
						Port:      80,
					},
				},
			},
			Metadata: core.Metadata{Name: "without-role", Namespace: "default"},
		}

		By("should error when no role provided")
		svcWithout, err := ec2.GetEc2Client(ec2.NewCredentialSpecFromEc2UpstreamSpec(withOutRole.UpstreamSpec.GetAwsEc2()), gloov1.SecretList{secret})
		Expect(err).NotTo(HaveOccurred())
		_, err = svcWithout.DescribeInstances(&ec2api.DescribeInstancesInput{})
		Expect(err).To(HaveOccurred())

		By("should succeed when role provided")
		svc, err := ec2.GetEc2Client(ec2.NewCredentialSpecFromEc2UpstreamSpec(withRole.UpstreamSpec.GetAwsEc2()), gloov1.SecretList{secret})
		Expect(err).NotTo(HaveOccurred())
		result, err := svc.DescribeInstances(&ec2api.DescribeInstancesInput{})
		Expect(err).NotTo(HaveOccurred())
		instances := ec2.GetInstancesFromDescription(result)
		// quick and dirty way to verify that we got some results
		// TODO(mitchdraft) validate output more thoroughly
		Expect(len(instances)).To(BeNumerically(">", 0))
	})

	// need to configure EC2 instances before running this
	XIt("be able to call upstream function", func() {
		err := envoyInstance.Run(testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		proxy := &gloov1.Proxy{
			Metadata: core.Metadata{
				Name:      "proxy",
				Namespace: "default",
			},
			Listeners: []*gloov1.Listener{{
				Name:        "listener",
				BindAddress: "::",
				BindPort:    defaults.HttpPort,
				ListenerType: &gloov1.Listener_HttpListener{
					HttpListener: &gloov1.HttpListener{
						VirtualHosts: []*gloov1.VirtualHost{{
							Name:    "virt1",
							Domains: []string{"*"},
							Routes: []*gloov1.Route{{
								Matcher: &gloov1.Matcher{
									PathSpecifier: &gloov1.Matcher_Prefix{
										Prefix: "/",
									},
								},
								Action: &gloov1.Route_RouteAction{
									RouteAction: &gloov1.RouteAction{
										Destination: &gloov1.RouteAction_Single{
											Single: &gloov1.Destination{
												DestinationType: &gloov1.Destination_Upstream{
													Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
												},
											},
										},
									},
								},
							}},
						}},
					},
				},
			}},
		}

		var opts clients.WriteOpts
		_, err = testClients.ProxyClient.Write(proxy, opts)
		Expect(err).NotTo(HaveOccurred())
		validateEc2Endpoint(defaults.HttpPort, "metrics")
	})

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		testClients = services.RunGateway(ctx, false)

		var err error
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())

		addCredentials()
		addUpstream()
	})
})
