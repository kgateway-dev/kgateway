package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/gloo/test/services/envoy"

	errors "github.com/rotisserie/eris"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/form3tech-oss/jwt-go"
	aws2 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/aws"
	"github.com/solo-io/gloo/test/helpers"
	"google.golang.org/protobuf/types/known/wrapperspb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	"github.com/solo-io/gloo/test/services"

	gw1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwdefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	transformationext "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	aws_plugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"

	"github.com/aws/aws-sdk-go/aws/credentials"
)

var _ = Describe("AWS Lambda", func() {
	const (
		region               = "us-east-1"
		webIdentityTokenFile = "AWS_WEB_IDENTITY_TOKEN_FILE"
		jwtPrivateKey        = "JWT_PRIVATE_KEY"
		awsRoleArn           = "AWS_ROLE_ARN"
	)

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *envoy.Instance
		secret        *gloov1.Secret
		upstream      *gloov1.Upstream
		httpClient    *http.Client
		runOptions    *services.RunOptions
	)

	BeforeEach(func() {
		httpClient = http.DefaultClient
		httpClient.Timeout = 10 * time.Second
		runOptions = &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableFds: false,
			},
		}
	})

	setupEnvoy := func(justGloo bool) {
		ctx, cancel = context.WithCancel(context.Background())

		envoyInstance = envoyFactory.NewInstance()

		runOptions.WhatToRun.DisableGateway = justGloo
		testClients = services.RunGlooGatewayUdsFds(ctx, runOptions)

		err := helpers.WriteDefaultGateways(defaults.GlooSystem, testClients.GatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")
	}

	type lambdaValidationParams struct {
		offset                          int
		envoyPort                       uint32
		requestBody                     string
		expectedSubstrings              []string
		requestHeaders, expectedHeaders http.Header
		requestUrl                      *url.URL
		expectedStatus                  *int
	}
	validateLambda := func(params lambdaValidationParams) {

		body := []byte("\"solo.io\"")
		if params.requestBody != "" {
			body = []byte(params.requestBody)
		}
		headers := http.Header{"Content-Type": {"application/octet-stream"}}
		if params.requestHeaders != nil {
			headers = params.requestHeaders
		}
		u := &url.URL{Scheme: "http", Host: fmt.Sprintf("localhost:%d", params.envoyPort), Path: "/1", RawQuery: "param_a=value_1&param_b=value_b"}
		if params.requestUrl != nil {
			u = params.requestUrl
		}
		expectedStatus := http.StatusOK
		if params.expectedStatus != nil {
			expectedStatus = *params.expectedStatus
		}

		EventuallyWithOffset(params.offset, func(g Gomega) {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)

			httpClient := &http.Client{
				Timeout: time.Minute * 5,
			}

			req := http.Request{
				Method: http.MethodPost,
				URL:    u,
				Header: headers,
				Body:   io.NopCloser(&buf),
			}

			req.Header.Add("x-header-a", "header_value_1")
			req.Header.Add("x-header-a", "header_value_2")
			req.Header.Add("x-header-b", "header_value_b")

			res, err := httpClient.Do(&req)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(res).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
				StatusCode: expectedStatus,
				Body:       testmatchers.ContainSubstrings(params.expectedSubstrings),
				Custom:     testmatchers.ContainHeaders(params.expectedHeaders),
			}))

		}, "30s", "1s").Should(Succeed())
	}
	validateLambdaUppercase := func(envoyPort uint32) {
		validateLambda(lambdaValidationParams{
			offset:             2,
			envoyPort:          envoyPort,
			expectedSubstrings: []string{"SOLO.IO"},
		})
	}

	addUpstream := func() {
		upstream = &gloov1.Upstream{
			Metadata: &core.Metadata{
				Namespace: "default",
				Name:      region,
			},
			UpstreamType: &gloov1.Upstream_Aws{
				Aws: &aws_plugin.UpstreamSpec{
					Region:    region,
					SecretRef: secret.Metadata.Ref(),
				},
			},
		}

		var opts clients.WriteOpts
		_, err := testClients.UpstreamClient.Write(upstream, opts)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() []*aws_plugin.LambdaFunctionSpec {
			us, err := testClients.UpstreamClient.Read(
				upstream.GetMetadata().Namespace,
				upstream.GetMetadata().Name,
				clients.ReadOpts{},
			)
			if err != nil {
				return nil
			}
			return us.GetAws().GetLambdaFunctions()
		}, "2m", "1s").Should(ContainElement(&aws_plugin.LambdaFunctionSpec{
			LogicalName:        "uppercase",
			LambdaFunctionName: "uppercase",
			Qualifier:          "$LATEST",
		}))
	}

	addCrossAccountUpstream := func() {
		upstream = &gloov1.Upstream{
			Metadata: &core.Metadata{
				Namespace: "default",
				Name:      "cross-account-us",
			},
			UpstreamType: &gloov1.Upstream_Aws{
				Aws: &aws_plugin.UpstreamSpec{
					Region:    region,
					SecretRef: secret.Metadata.Ref(),
					// this is a separate account ID from the one that all other lambda
					// functions tested in this file are in
					AwsAccountId: "986112284769",
					LambdaFunctions: []*aws_plugin.LambdaFunctionSpec{
						{
							LogicalName:        "resource-based-cross-account-hello",
							LambdaFunctionName: "resource-based-cross-account-hello",
						},
					},
				},
			},
		}

		var opts clients.WriteOpts
		_, err := testClients.UpstreamClient.Write(upstream, opts)
		Expect(err).NotTo(HaveOccurred())

		// wait for the upstream to be created
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.UpstreamClient.Read(upstream.Metadata.Namespace, upstream.Metadata.Name, clients.ReadOpts{})
		}, "30s", "1s")
	}

	createProxy := func(unwrapAsApiGateway, requestTransformation, responseTransformation bool, logicalName string) {
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
												DestinationSpec: &gloov1.DestinationSpec{
													DestinationType: &gloov1.DestinationSpec_Aws{
														Aws: &aws_plugin.DestinationSpec{
															LogicalName:            logicalName,
															UnwrapAsApiGateway:     unwrapAsApiGateway,
															RequestTransformation:  requestTransformation,
															ResponseTransformation: responseTransformation,
														},
													},
												},
											},
										},
									},
								},
							}},
						},
						},
					},
				},
			}},
		}

		var opts clients.WriteOpts
		_, err := testClients.ProxyClient.Write(proxy, opts)
		Expect(err).NotTo(HaveOccurred())

		// wait for proxy to be accepted
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(proxy.Metadata.Namespace, proxy.Metadata.Name, clients.ReadOpts{})
		}, "30s", "1s")

	}

	testProxy := func() {
		err := envoyInstance.RunWithRoleAndRestXds(envoy.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		createProxy(false, false, false, "uppercase")
		validateLambdaUppercase(envoyInstance.HttpPort)
	}

	testProxyWithResponseTransform := func() {
		err := envoyInstance.RunWithRoleAndRestXds(envoy.DefaultProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		createProxy(false, false, true, "contact-form")
		validateLambda(lambdaValidationParams{
			offset:             1,
			envoyPort:          envoyInstance.HttpPort,
			expectedSubstrings: []string{`<meta http-equiv="Content-Type" content="text/html; charset=UTF-8"/>`},
		})
	}

	testProxyWithRequestTransform := func() {
		err := envoyInstance.RunWithRole(envoy.DefaultProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		createProxy(false, true, false, "dumpContext")
		validateLambda(lambdaValidationParams{
			offset:    1,
			envoyPort: envoyInstance.HttpPort,
			expectedSubstrings: []string{`\"body\": \"\\\"solo.io\\\"\", \"headers\": `,
				`\"queryString\": \"param_a=value_1&param_b=value_b\"`,
				`\"path\": \"/1\"`,
				`\"httpMethod\": \"POST\"`},
		})
	}

	testProxyWithUnwrapAsApiGateway := func() {
		err := envoyInstance.RunWithRole(envoy.DefaultProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		createProxy(true, false, false, "echo")
		expectedStatus := 201
		// need querystring, multivaluequerystring
		validateLambda(lambdaValidationParams{
			offset:             1,
			envoyPort:          envoyInstance.HttpPort,
			requestBody:        `{"headers":{"Content-Type":"application/test"}, "body":"solo.io", "multiValueHeaders":{"x-header":["value-1", "value-2"]}, "statusCode":201, "queryStringParameters":{"param_a":"value_2", "param_b":"value_b"}, "multiValueQueryStringParameters":{"param_a":["value_1", "value_2"]}}`,
			expectedSubstrings: []string{"solo.io"},
			expectedHeaders:    http.Header{"Content-Type": {"application/test"}, "X-Header": {"value-1,value-2"}},
			expectedStatus:     &expectedStatus,
		})
	}

	testProxyWithRequestAndResponseTransforms := func() {
		err := envoyInstance.RunWithRole(envoy.DefaultProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		createProxy(false, true, true, "dumpContext")
		validateLambda(lambdaValidationParams{
			offset:             1,
			envoyPort:          envoyInstance.HttpPort,
			expectedSubstrings: []string{`"\"solo.io\""`},
		})
	}

	testProxyWithCrossAccountLambda := func() {
		err := envoyInstance.RunWithRole(envoy.DefaultProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		createProxy(false, false, false, "resource-based-cross-account-hello")
		validateLambda(lambdaValidationParams{
			offset:             1,
			envoyPort:          envoyInstance.HttpPort,
			expectedSubstrings: []string{`"\"Hello from Lambda!\""`},
		})
	}

	testLambdaWithVirtualService := func() {
		err := envoyInstance.RunWithRoleAndRestXds("gloo-system~"+gwdefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		vs := &gw1.VirtualService{
			Metadata: &core.Metadata{
				Name:      "app",
				Namespace: "gloo-system",
			},
			VirtualHost: &gw1.VirtualHost{
				Domains: []string{"*"},
				Routes: []*gw1.Route{{
					Action: &gw1.Route_RouteAction{
						RouteAction: &gloov1.RouteAction{
							Destination: &gloov1.RouteAction_Single{
								Single: &gloov1.Destination{
									DestinationType: &gloov1.Destination_Upstream{
										Upstream: upstream.Metadata.Ref(),
									},
									DestinationSpec: &gloov1.DestinationSpec{
										DestinationType: &gloov1.DestinationSpec_Aws{
											Aws: &aws_plugin.DestinationSpec{
												LogicalName: "uppercase",
											},
										},
									},
								},
							},
						},
					},
				}},
			},
		}

		var opts clients.WriteOpts
		_, err = testClients.VirtualServiceClient.Write(vs, opts)
		Expect(err).NotTo(HaveOccurred())

		validateLambdaUppercase(envoyInstance.HttpPort)
	}

	testLambdaTransformations := func() {
		// don't generate request id, so that the returned body is predictable (see the MatchJson below).
		gateway, err := testClients.GatewayClient.Read(defaults.GlooSystem, gwdefaults.GatewayProxyName, clients.ReadOpts{})
		gateway.GetHttpGateway().Options = &gloov1.HttpListenerOptions{
			HttpConnectionManagerSettings: &hcm.HttpConnectionManagerSettings{
				GenerateRequestId: wrapperspb.Bool(false),
			},
		}
		_, err = testClients.GatewayClient.Write(gateway, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())

		err = envoyInstance.RunWithRoleAndRestXds(defaults.GlooSystem+"~"+gwdefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())

		prepVs := func(addResp bool) {
			path := "/transforms-req-test"
			if addResp {
				path = "/transforms-resp-test"
			}

			vs := &gw1.VirtualService{
				Metadata: &core.Metadata{
					Name:      "app",
					Namespace: "gloo-system",
				},
				VirtualHost: &gw1.VirtualHost{
					Domains: []string{"*"},
					Routes: []*gw1.Route{{
						Options: &gloov1.RouteOptions{
							Transformations: &transformation.Transformations{
								ResponseTransformation: &transformation.Transformation{
									TransformationType: &transformation.Transformation_TransformationTemplate{
										TransformationTemplate: &transformationext.TransformationTemplate{
											Headers: map[string]*transformationext.InjaTemplate{
												"foo": {
													Text: "bar",
												},
											},
										},
									},
								},
							},
						},
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: path,
								},
							},
						},
						Action: &gw1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: upstream.Metadata.Ref(),
										},
										DestinationSpec: &gloov1.DestinationSpec{
											DestinationType: &gloov1.DestinationSpec_Aws{
												Aws: &aws_plugin.DestinationSpec{
													LogicalName:            "echo",
													RequestTransformation:  true,
													ResponseTransformation: addResp,
												},
											},
										},
									},
								},
							},
						},
					}},
				},
			}

			var opts clients.WriteOpts
			_, err = testClients.VirtualServiceClient.Write(vs, opts)
			Expect(err).NotTo(HaveOccurred())
		}

		By("sending a request with no response transformation")
		prepVs(false)
		var res *http.Response
		var body []byte
		path := "transforms-req-test"
		waitForLambdaAndGetBody := func() error {
			req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d/%s?foo=bar", "localhost", envoyInstance.HttpPort, path), bytes.NewBufferString(`"test"`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/octet-stream")
			req.Host = "test"
			res, err = httpClient.Do(req)
			if err != nil {
				return err
			}
			if res.StatusCode != http.StatusOK {
				res.Body.Close()
				return errors.New(fmt.Sprintf("%v is not OK", res.StatusCode))
			}

			defer res.Body.Close()
			body, err = io.ReadAll(res.Body)
			Expect(err).NotTo(HaveOccurred())
			return nil
		}
		EventuallyWithOffset(1, waitForLambdaAndGetBody, "5m", "1s").ShouldNot(HaveOccurred())

		Expect(res.Header).To(HaveKeyWithValue("Foo", ContainElement("bar")))
		// see that the AWS request transform applied - this means that the lambda will get a json body
		// and will return its error response - not a string
		Expect(string(body)).To(MatchJSON(`{"body":"\"test\"","headers":{":authority":"test",":method":"POST",":path":"/transforms-req-test?foo=bar",":scheme":"http","accept-encoding":"gzip","content-length":"6","content-type":"application/octet-stream","user-agent":"Go-http-client/1.1","x-forwarded-proto":"http"},"httpMethod":"POST","multiValueHeaders":{},"multiValueQueryStringParameters":{},"path":"/transforms-req-test","queryString":"foo=bar","queryStringParameters":{"foo":"bar"}}`))

		By("sending a request with response transformation")
		path = "transforms-resp-test"
		err = testClients.VirtualServiceClient.Delete("gloo-system", "app", clients.DeleteOpts{})
		Expect(err).NotTo(HaveOccurred())
		prepVs(true)
		EventuallyWithOffset(1, waitForLambdaAndGetBody, "5m", "1s").ShouldNot(HaveOccurred())

		Expect(res.Header).To(HaveKeyWithValue("Foo", ContainElement("bar")))
		// response transform restores the body
		Expect(string(body)).To(Equal(`"test"`))

	}

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	Context("Basic Auth", func() {

		addBasicCredentials := func() {

			localAwsCredentials := credentials.NewSharedCredentials("", "")
			v, err := localAwsCredentials.Get()
			if err != nil {
				Fail("no AWS creds available")
			}
			var opts clients.WriteOpts

			accesskey := v.AccessKeyID
			secretkey := v.SecretAccessKey

			secret = &gloov1.Secret{
				Metadata: &core.Metadata{
					Namespace: "default",
					Name:      region,
				},
				Kind: &gloov1.Secret_Aws{
					Aws: &gloov1.AwsSecret{
						AccessKey: accesskey,
						SecretKey: secretkey,
					},
				},
			}

			_, err = testClients.SecretClient.Write(secret, opts)
			Expect(err).NotTo(HaveOccurred())
		}
		Context("Without gateway translation", func() {
			BeforeEach(func() {
				setupEnvoy(true)
				addBasicCredentials()
				addUpstream()
			})

			It("should be able to call lambda", testProxy)

			It("should be able to call lambda with response transform", testProxyWithResponseTransform)

			It("should be able to call lambda with unwrapAsApiGateway", testProxyWithUnwrapAsApiGateway)

			It("should be able to call lambda with request transform", testProxyWithRequestTransform)

			It("should be able to call lambda with request and response transforms", testProxyWithRequestAndResponseTransforms)
		})
		Context("With gateway translation", func() {
			BeforeEach(func() {
				setupEnvoy(false)
				addBasicCredentials()
				addUpstream()
			})
			It("should be able to call lambda via gateway", testLambdaWithVirtualService)

			It("should be able to call lambda transformation and regular transformation", testLambdaTransformations)
		})
		Context("Resource-based cross-account lambda", func() {
			BeforeEach(func() {
				runOptions.WhatToRun.DisableFds = true
				setupEnvoy(true)
				addBasicCredentials()
				addCrossAccountUpstream()
			})

			It("should be able to interact with resource-based cross-account lambda", testProxyWithCrossAccountLambda)
		})
	})
	Context("Temporary Credentials", func() {

		addCredentials := func() {
			localAwsCredentials := credentials.NewSharedCredentials("", "")
			sess, err := session.NewSession(&aws.Config{Region: aws.String(region), Credentials: localAwsCredentials})
			if err != nil {
				Fail("no AWS creds available")
			}
			stsClient := sts.New(sess)
			result, err := stsClient.GetSessionToken(&sts.GetSessionTokenInput{})
			Expect(err).NotTo(HaveOccurred())

			var opts clients.WriteOpts
			secret = &gloov1.Secret{
				Metadata: &core.Metadata{
					Namespace: "default",
					Name:      region,
				},
				Kind: &gloov1.Secret_Aws{
					Aws: &gloov1.AwsSecret{
						AccessKey:    *result.Credentials.AccessKeyId,
						SecretKey:    *result.Credentials.SecretAccessKey,
						SessionToken: *result.Credentials.SessionToken,
					},
				},
			}

			_, err = testClients.SecretClient.Write(secret, opts)
			Expect(err).NotTo(HaveOccurred())
		}
		Context("No gateway translation", func() {

			BeforeEach(func() {
				setupEnvoy(true)
				addCredentials()
				addUpstream()
			})

			It("should be able to call lambda", testProxy)

			It("should be able to call lambda with response transform", testProxyWithResponseTransform)

			It("should be able to call lambda with request transform", testProxyWithRequestTransform)

			It("should be able to call lambda with request and response transforms", testProxyWithRequestAndResponseTransforms)
		})
		Context("With gateawy translation", func() {
			BeforeEach(func() {
				setupEnvoy(false)
				addCredentials()
				addUpstream()
			})

			It("should be able to call lambda via gateway", testLambdaWithVirtualService)

			It("should be able to call lambda transformation and regular transformation", testLambdaTransformations)
		})
	})
	Context("AssumeRoleWithWebIdentity Credentials", func() {

		var (
			tmpFile *os.File
		)

		addCredentialsSts := func() {
			roleArn := "arn:aws:iam::802411188784:role/gloo-edge-e2e-sts"
			jwtKey := os.Getenv(jwtPrivateKey)
			if jwtKey == "" {
				Fail(fmt.Sprintf("Token location unset, set via %s", jwtPrivateKey))
			}

			// Need to store the private key in base 64 otherwise the newlines get lost in the env var
			data, err := base64.StdEncoding.DecodeString(jwtKey)
			Expect(err).NotTo(HaveOccurred())

			privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(data)
			Expect(err).NotTo(HaveOccurred())

			now := time.Now()

			tokenToSign := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
				"sub":   "1234567890",
				"name":  "Solo Test User",
				"admin": true,
				"iat":   now.Unix(),
				"exp":   now.Add(time.Minute * 10).Unix(),
				"nbf":   now.Unix(),
				"iss":   "https://fake-oidc.solo.io",
				"aud":   "sts.amazonaws.com",
				"kid":   "XwCb60dEzG6QF4-5iCwFRE1w1hP_VEoy3JWcokISRp4",
			})

			signedJwt, err := tokenToSign.SignedString(privateKey)
			Expect(err).NotTo(HaveOccurred())

			tmpFile, err = os.CreateTemp("/tmp", "")
			Expect(err).NotTo(HaveOccurred())
			defer tmpFile.Close()

			_, err = tmpFile.Write([]byte(signedJwt))
			Expect(err).NotTo(HaveOccurred())

			// Have to set these values for tests which use the envoy binary
			os.Setenv(webIdentityTokenFile, tmpFile.Name())
			os.Setenv(awsRoleArn, roleArn)

			envoyInstance.DockerOptions = envoy.DockerOptions{
				Volumes: []string{fmt.Sprintf("%s:%s", tmpFile.Name(), tmpFile.Name())},
				Env:     []string{webIdentityTokenFile, awsRoleArn},
			}
		}

		addUpstreamSts := func() {
			upstream = &gloov1.Upstream{
				Metadata: &core.Metadata{
					Namespace: "default",
					Name:      region,
				},
				UpstreamType: &gloov1.Upstream_Aws{
					Aws: &aws_plugin.UpstreamSpec{
						Region: region,
					},
				},
			}

			var opts clients.WriteOpts
			_, err := testClients.UpstreamClient.Write(upstream, opts)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() []*aws_plugin.LambdaFunctionSpec {
				us, err := testClients.UpstreamClient.Read(
					upstream.GetMetadata().Namespace,
					upstream.GetMetadata().Name,
					clients.ReadOpts{},
				)
				if err != nil {
					return nil
				}
				return us.GetAws().GetLambdaFunctions()
			}, "2m", "1s").Should(ContainElement(&aws_plugin.LambdaFunctionSpec{
				LogicalName:        "uppercase",
				LambdaFunctionName: "uppercase",
				Qualifier:          "$LATEST",
			}))
		}

		setupEnvoySts := func(justGloo bool) {
			ctx, cancel = context.WithCancel(context.Background())

			envoyInstance = envoyFactory.NewInstance()

			ns := defaults.GlooSystem
			ro := &services.RunOptions{
				NsToWrite: ns,
				NsToWatch: []string{"default", ns},
				WhatToRun: services.What{
					DisableGateway: justGloo,
				},
				Settings: &gloov1.Settings{
					Gloo: &gloov1.GlooOptions{
						AwsOptions: &gloov1.GlooOptions_AWSOptions{
							CredentialsFetcher: &gloov1.GlooOptions_AWSOptions_ServiceAccountCredentials{
								ServiceAccountCredentials: &aws2.AWSLambdaConfig_ServiceAccountCredentials{
									Cluster: "aws_sts_cluster",
									Uri:     "sts.amazonaws.com",
								},
							},
						},
					},
				},
			}
			testClients = services.RunGlooGatewayUdsFds(ctx, ro)

			err := helpers.WriteDefaultGateways(defaults.GlooSystem, testClients.GatewayClient)
			Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")
		}

		BeforeEach(func() {
			testutils.ValidateRequirementsAndNotifyGinkgo(testutils.DefinedEnv(jwtPrivateKey))
		})

		AfterEach(func() {
			if tmpFile != nil {
				os.Remove(tmpFile.Name())
			}
			os.Unsetenv(webIdentityTokenFile)
		})
		Context("No gateway translation ", func() {
			BeforeEach(func() {
				setupEnvoySts(true)
				addCredentialsSts()
				addUpstreamSts()
			})
			/*
			 * these tests can start failing if certs get rotated underneath us.
			 * the fix is to update the rotated thumbprint on our fake AWS OIDC per
			 * https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_create_oidc_verify-thumbprint.html
			 */
			It("should be able to call lambda", testProxy)

			It("should be able to call lambda with response transform", testProxyWithResponseTransform)

			It("should be able to call lambda with request transform", testProxyWithRequestTransform)

			It("should be able to call lambda with request and response transforms", testProxyWithRequestAndResponseTransforms)
		})
		Context("With gateway translation", func() {
			BeforeEach(func() {
				setupEnvoySts(false)
				addCredentialsSts()
				addUpstreamSts()
			})
			It("should be able to call lambda via gateway", testLambdaWithVirtualService)

			It("should be able to call lambda transformation and regular transformation", testLambdaTransformations)
		})
	})

})
