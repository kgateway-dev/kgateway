package e2e_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/aws/aws-sdk-go/aws/credentials"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws/ec2"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

/*
# Configure an EC2 instance for this test
- Do this if this test ever starts to fail because the EC2 instance that it tests against has become unavailable.

- Provision an EC2 instance
  - Use an "amazon linux" image
  - Configure the security group to allow http traffic on port 80

- Tag your instance with the following tags
  - svc: worldwide-hello

- Set up your EC2 instance
  - ssh into your instance
  - download a demo app: an http response code echo app
    - this app responds to requests with the corresponding response code
      - ex: http://<my-instance-ip>/?code=404 produces a `404` response
  - make the app executable
  - run it in the background

```bash
wget https://mitch-solo-public.s3.amazonaws.com/echoapp2
chmod +x echoapp2
sudo ./echoapp2 --port 80 &
```
- Note: other dummy webservers will work fine - you may just need to update the path of the request
  - Currently, we call the /metrics path during our tests

- Verify that you can reach the app
  - `curl` the app, you should see a help menu for the app
```bash
curl http://<instance-public-ip>/
```
*/

var _ = Describe("AWS EC2 Plugin utils test", func() {
	if testutils.ShouldSkipTempDisabledTests() {
		return
	}
	const (
		region     = "us-east-1"
		awsRoleArn = "AWS_ROLE_ARN"
	)

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *envoy.Instance
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
		roleArn = os.Getenv(awsRoleArn)
		if roleArn == "" {
			Skip("no AWS role ARN available")
		}
		var opts clients.WriteOpts

		accessKey := v.AccessKeyID
		secretKey := v.SecretAccessKey

		secret = &gloov1.Secret{
			Metadata: &core.Metadata{
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
			Metadata: &core.Metadata{
				Namespace: "default",
				Name:      region,
			},
			UpstreamType: &gloov1.Upstream_AwsEc2{
				AwsEc2: &glooec2.UpstreamSpec{
					Region:    region,
					SecretRef: secretRef,
					RoleArn:   roleArn,
					Filters: []*glooec2.TagFilter{
						{
							Spec: &glooec2.TagFilter_KvPair_{
								KvPair: &glooec2.TagFilter_KvPair{
									Key:   "svc",
									Value: "worldwide-hello",
								},
							},
						},
					},
					PublicIp: true,
					Port:     80,
				},
			},
		}

		var opts clients.WriteOpts
		_, err := testClients.UpstreamClient.Write(upstream, opts)
		Expect(err).NotTo(HaveOccurred())
	}

	validateUrl := func(url, substring string) {
		Eventually(func() (string, error) {
			res, err := http.Get(url)
			if err != nil {
				return "", eris.Wrapf(err, "unable to call GET")
			}
			if res.StatusCode != http.StatusOK {
				return "", eris.New(fmt.Sprintf("%v is not OK", res.StatusCode))
			}

			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return "", eris.Wrapf(err, "unable to read body")
			}

			return string(body), nil
		}, "10s", "1s").Should(ContainSubstring(substring))
	}

	validateEc2Endpoint := func(envoyPort uint32, substring string) {
		// first make sure that the instance is ready (to avoid false negatives)
		By("verifying instance is ready - if this failed, you may need to restart the EC2 instance")
		// Stitch the url together to avoid bot spam
		// The IP address corresponds to the public ip of an EC2 instance managed by Solo.io for the purpose of
		// verifying that the EC2 upstream works as expected.
		// The port is where the app listens for connections. The instance has been configured with an inbound traffic
		// rule that allows port 80.
		// TODO[test enhancement] - create an EC2 instance on demand (or auto-skip the test) if the expected instance is unavailable
		// See notes in the header of this file for instructions on how to restore the instance
		ec2Port := 80
		// This is an Elastic IP in us-east-1 and can be reassigned if the instance ever goes down
		ec2Url := fmt.Sprintf("http://%v:%v/metrics", strings.Join([]string{"100", "24", "224", "6"}, "."), ec2Port)
		validateUrl(ec2Url, substring)

		// do the actual verification
		By("verifying Gloo has routed to the instance")
		gatewayUrl := fmt.Sprintf("http://%v:%v/metrics", "localhost", envoyPort)
		validateUrl(gatewayUrl, substring)
	}

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	// NOTE: you need to configure EC2 instances before running this
	It("be able to call upstream function", func() {
		err := envoyInstance.RunWithRoleAndRestXds(envoy.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		proxy := &gloov1.Proxy{
			Metadata: &core.Metadata{
				Name:      "proxy",
				Namespace: "default",
			},
			Listeners: []*gloov1.Listener{{
				Name:        "listener",
				BindAddress: "::",
				BindPort:    envoyInstance.HttpPort,
				ListenerType: &gloov1.Listener_HttpListener{
					HttpListener: &gloov1.HttpListener{
						VirtualHosts: []*gloov1.VirtualHost{{
							Name:    "virt1",
							Domains: []string{"*"},
							Routes: []*gloov1.Route{{
								Action: &gloov1.Route_RouteAction{
									RouteAction: &gloov1.RouteAction{
										Destination: &gloov1.RouteAction_Single{
											Single: &gloov1.Destination{
												DestinationType: &gloov1.Destination_Upstream{
													Upstream: upstream.Metadata.Ref(),
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

		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
		})

		validateEc2Endpoint(envoyInstance.HttpPort, "Counts")
	})

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		envoyInstance = envoyFactory.NewInstance()

		runOptions := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableGateway: true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, runOptions)

		addCredentials()
		addUpstream()
	})
})
