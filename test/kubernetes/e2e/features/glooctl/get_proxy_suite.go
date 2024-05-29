package glooctl

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/ghodss/yaml"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

var (
	yamlSeparator = regexp.MustCompile("\n---\n")
)

// getProxySuite contains the set of tests to validate the behavior of `glooctl get proxy`
type getProxySuite struct {
	suite.Suite

	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewGetProxySuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &getProxySuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *getProxySuite) SetupSuite() {
	// apply backend resources that other manifests will depend on
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, backendManifestFile)
	s.Require().NoError(err, "can apply backend manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, nginxSvc, nginxPod, nginxUpstream)

	// apply edge virtual services
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, edgeRoutesManifestFile)
	s.Require().NoError(err, "can apply edge routes manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, edgeVs1, edgeVs2)

	// apply edge gateways; these are applied in a separate manifest because they need to be applied in
	// the write namespace in order for gloo to process them
	ns := s.testInstallation.Metadata.InstallNamespace
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, edgeGatewaysManifestFile, "-n", ns)
	s.Require().NoError(err, "can apply edge gateways manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, getEdgeGateway1(ns), getEdgeGateway2(ns))

	// apply kube gateways and httproutes
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, kubeGatewaysManifestFile)
	s.Require().NoError(err, "can apply kube gateways manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, kubeGateway1, kubeRoute1, kubeGateway2, kubeRoute2)

	// wait for the proxies to get created
	s.testInstallation.Assertions.Gomega.Eventually(func(g Gomega) {
		output, err := s.testInstallation.Actions.Glooctl().GetProxy(s.ctx, "-n", ns, "-o", "kube-yaml")
		g.Expect(err).NotTo(HaveOccurred())
		proxies, err := parseProxyOutput(output)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(proxies).To(ConsistOf(
			matchers.HaveNameAndNamespace(edgeProxy1Name, ns),
			matchers.HaveNameAndNamespace(edgeProxy2Name, ns),
			matchers.HaveNameAndNamespace(edgeDefaultProxyName, ns),
			matchers.HaveNameAndNamespace(kubeProxy1Name, ns),
			matchers.HaveNameAndNamespace(kubeProxy2Name, ns),
		))
	}).
		WithContext(s.ctx).
		WithTimeout(time.Second*10).
		WithPolling(time.Second).
		Should(Succeed(), "proxies should be available to query")
}

func (s *getProxySuite) TearDownSuite() {
	// delete manifests in the reverse order that they were applied
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, kubeGatewaysManifestFile)
	s.NoError(err, "can delete kube gateways manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, kubeGateway1, kubeRoute1, kubeGateway2, kubeRoute2)

	ns := s.testInstallation.Metadata.InstallNamespace
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, edgeGatewaysManifestFile, "-n", ns)
	s.NoError(err, "can delete edge gateways manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, getEdgeGateway1(ns), getEdgeGateway2(ns))

	// we are calling delete with retries here because there is some delay between Gateways being deleted and
	// gloo picking up the updates in its input snapshot, causing VS deletion to fail in the meantime (gloo
	// thinks there are still Gateways referencing the VS)
	err = retry.Do(func() error {
		return s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, edgeRoutesManifestFile)
	},
		retry.LastErrorOnly(true),
		retry.Delay(1*time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(8))
	s.NoError(err, "can delete edge routes manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, edgeVs1, edgeVs2)

	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, backendManifestFile)
	s.NoError(err, "can delete backend manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, nginxSvc, nginxPod, nginxUpstream)
}

func (s *getProxySuite) TestGetProxy() {
	// test `glooctl get proxy` with various args. set the output type to kube-yaml for each request, so that we can parse the response into Proxies
	outputTypeArgs := []string{"-o", "kube-yaml"}
	for _, testCase := range getTestCases(s.testInstallation.Metadata.InstallNamespace) {
		s.Run(testCase.name, func() {
			output, err := s.testInstallation.Actions.Glooctl().GetProxy(s.ctx, slices.Concat(testCase.args, outputTypeArgs)...)
			Expect(err).To(testCase.errorMatcher)
			proxies, err := parseProxyOutput(output)
			s.NoError(err)
			Expect(proxies).To(testCase.proxiesMatcher)
		})
	}
}

type getProxyTestCase struct {
	name           string
	args           []string
	errorMatcher   types.GomegaMatcher
	proxiesMatcher types.GomegaMatcher
}

func getTestCases(installNamespace string) []getProxyTestCase {
	return []getProxyTestCase{
		{
			// glooctl get proxy (no args) => should return error (defaults to gloo-system ns)
			name:           "InvalidNamespace",
			args:           []string{},
			errorMatcher:   MatchError(ContainSubstring("Gloo installation namespace does not exist")),
			proxiesMatcher: gstruct.Ignore(),
		},
		{
			// glooctl get proxy -n <installNs> => should get all proxies
			name:         "AllProxiesInNamespace",
			args:         []string{"-n", installNamespace},
			errorMatcher: BeNil(),
			proxiesMatcher: ConsistOf(
				matchers.HaveNameAndNamespace(edgeProxy1Name, installNamespace),
				matchers.HaveNameAndNamespace(edgeProxy2Name, installNamespace),
				matchers.HaveNameAndNamespace(edgeDefaultProxyName, installNamespace),
				matchers.HaveNameAndNamespace(kubeProxy1Name, installNamespace),
				matchers.HaveNameAndNamespace(kubeProxy2Name, installNamespace),
			),
		},
		{
			// glooctl get proxy -n <installNs> --name proxy1 => should get proxy with name
			name:         "ProxyName",
			args:         []string{"-n", installNamespace, "--name", "proxy1"},
			errorMatcher: BeNil(),
			proxiesMatcher: ConsistOf(
				matchers.HaveNameAndNamespace(edgeProxy1Name, installNamespace),
			),
		},
		{
			// glooctl get proxy -n <installNs> --name nonexistent => should return error
			name:           "InvalidProxyName",
			args:           []string{"-n", installNamespace, "--name", "nonexistent"},
			errorMatcher:   MatchError(ContainSubstring(fmt.Sprintf("%s.%s does not exist", installNamespace, "nonexistent"))),
			proxiesMatcher: gstruct.Ignore(),
		},
		{
			// glooctl get proxy -n <installNs> --name proxy1 --kube => should ignore kube flag, and return proxy with name
			// (even though it's an edge proxy)
			name:         "ProxyNameIgnoreSelector",
			args:         []string{"-n", installNamespace, "--name", "proxy1", "--kube"},
			errorMatcher: BeNil(),
			proxiesMatcher: ConsistOf(
				matchers.HaveNameAndNamespace(edgeProxy1Name, installNamespace),
			),
		},
		{
			// glooctl get proxy -n <installNs> --edge => should return only edge proxies
			name:         "EdgeProxies",
			args:         []string{"-n", installNamespace, "--edge"},
			errorMatcher: BeNil(),
			proxiesMatcher: ConsistOf(
				matchers.HaveNameAndNamespace(edgeProxy1Name, installNamespace),
				matchers.HaveNameAndNamespace(edgeProxy2Name, installNamespace),
				matchers.HaveNameAndNamespace(edgeDefaultProxyName, installNamespace),
			),
		},
		{
			// glooctl get proxy -n <installNs> --kube => should return only kube proxies
			name:         "KubeProxies",
			args:         []string{"-n", installNamespace, "--kube"},
			errorMatcher: BeNil(),
			proxiesMatcher: ConsistOf(
				matchers.HaveNameAndNamespace(kubeProxy1Name, installNamespace),
				matchers.HaveNameAndNamespace(kubeProxy2Name, installNamespace),
			),
		},
		{
			// glooctl get proxy -n <installNs> --edge --kube => should return both kube and edge proxies
			name:         "EdgeAndKubeProxies",
			args:         []string{"-n", installNamespace, "--edge", "--kube"},
			errorMatcher: BeNil(),
			proxiesMatcher: ConsistOf(
				matchers.HaveNameAndNamespace(edgeProxy1Name, installNamespace),
				matchers.HaveNameAndNamespace(edgeProxy2Name, installNamespace),
				matchers.HaveNameAndNamespace(edgeDefaultProxyName, installNamespace),
				matchers.HaveNameAndNamespace(kubeProxy1Name, installNamespace),
				matchers.HaveNameAndNamespace(kubeProxy2Name, installNamespace),
			),
		},
	}
}

func parseProxyOutput(output string) ([]*gloov1.Proxy, error) {
	var proxies []*gloov1.Proxy

	// strip off any glooctl output before the apiVersion
	start := strings.Index(output, "apiVersion:")
	if start < 0 {
		// there are no proxies
		return proxies, nil
	}
	proxiesYaml := output[start:]
	splitProxiesYaml := yamlSeparator.Split(proxiesYaml, -1)
	for _, proxyYaml := range splitProxiesYaml {
		proxy := &gloov1.Proxy{}
		err := yaml.Unmarshal([]byte(proxyYaml), &proxy)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
	}
	return proxies, nil
}
