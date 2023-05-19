package knative_test

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/solo-io/gloo/jobs/pkg/certgen"
	"github.com/solo-io/gloo/jobs/pkg/kube"
	"github.com/solo-io/gloo/jobs/pkg/run"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/k8s-utils/testutils/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	v1 "k8s.io/api/core/v1"
)

var _ = Describe("Kube2e: Knative-Ingress with manual TLS enabled", func() {

	AfterEach(func() {
		if err := deleteTLSSecret(); err != nil {
			log.Warnf("teardown failed, knative tls secret may still be present %v", err)
		}
		if err := deleteKnativeTestService(knativeTLSTestServiceFile()); err != nil {
			log.Warnf("teardown failed, knative test service may still be present %v", err)
		}
	})

	It("works", func() {
		addTLSSecret()
		deployKnativeTestService(knativeTLSTestServiceFile())

		clusterIP := getClusterIP()
		ingressPort := 443
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "https",
			Path:              "/",
			Method:            "GET",
			Host:              "helloworld-go.default.example.com",
			Service:           clusterIP,
			Port:              ingressPort,
			ConnectionTimeout: 1,
			Verbose:           true,
			SelfSigned:        true,
			Sni:               "helloworld-go.default.example.com",
		}, "Hello Go Sample v1!", 1, time.Minute*2, 1*time.Second)
	})

	It("works when the secret is added after the service which points to it", func() {

		deployKnativeTestService(knativeTLSTestServiceFile())
		// Allow the service a few seconds to be created
		time.Sleep(3 * time.Second)
		addTLSSecret()

		clusterIP := getClusterIP()
		ingressPort := 443
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "https",
			Path:              "/",
			Method:            "GET",
			Host:              "helloworld-go.default.example.com",
			Service:           clusterIP,
			Port:              ingressPort,
			ConnectionTimeout: 1,
			Verbose:           true,
			SelfSigned:        true,
			Sni:               "helloworld-go.default.example.com",
		}, "Hello Go Sample v1!", 1, time.Minute*2, 1*time.Second)
	})

})

func addTLSSecret() {
	opts := run.Options{
		SecretName:                  "my-knative-tls-secret",
		SecretNamespace:             defaults.DefaultValue,
		SvcName:                     "knative-external-proxy",
		SvcNamespace:                testHelper.InstallNamespace,
		ServerKeySecretFileName:     v1.TLSPrivateKeyKey,
		ServerCertSecretFileName:    v1.TLSCertKey,
		ServerCertAuthorityFileName: v1.ServiceAccountRootCAKey,
	}
	certs, err := certgen.GenCerts(opts.SvcName, opts.SvcNamespace)
	Expect(err).NotTo(HaveOccurred(), "it should generate the cert")
	kubeClient := helpers.MustKubeClient()

	caCert := append(certs.ServerCertificate, certs.CaCertificate...)
	secretConfig := kube.TlsSecret{
		SecretName:         opts.SecretName,
		SecretNamespace:    opts.SecretNamespace,
		PrivateKeyFileName: opts.ServerKeySecretFileName,
		CertFileName:       opts.ServerCertSecretFileName,
		CaBundleFileName:   opts.ServerCertAuthorityFileName,
		Cert:               caCert,
		PrivateKey:         certs.ServerCertKey,

		// We intentionally do not provide a CaBundle here. Due to the way Gloo works, if we provide a CaBundle,
		// we assume that we need to verify the identity of the client, and expect a client certificate to be
		// passed in the request. By not including the CaBundle we are testing TLS and ensuring that only
		// the client verifies the identity of the server.
		// CaBundle:           certs.CaCertificate,
	}

	_, err = kube.CreateTlsSecret(context.Background(), kubeClient, secretConfig)
	Expect(err).NotTo(HaveOccurred(), "it should create the tls secret")
}

func deleteTLSSecret() error {
	kubectlArgs := strings.Split("kubectl delete secret my-knative-tls-secret", " ")
	err := exec.RunCommandInput("", testHelper.RootDir, true, kubectlArgs...)
	if err != nil {
		return err
	}
	return nil
}

func getClusterIP() string {
	kubectlArgs := strings.Split("kubectl get services -n "+testHelper.InstallNamespace+" knative-external-proxy -o jsonpath='{.spec.clusterIP}'", " ")
	clusterIP, err := exec.RunCommandInputOutput("", testHelper.RootDir, true, kubectlArgs...)
	Expect(err).NotTo(HaveOccurred())
	return strings.ReplaceAll(clusterIP, "'", "")
}

func knativeTLSTestServiceFile() string {
	return filepath.Join(testHelper.RootDir, "test", "kube2e", "knative", "artifacts", "knative-hello-service-tls.yaml")
}
