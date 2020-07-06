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
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/go-utils/testutils/helper"

	"github.com/solo-io/go-utils/log"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	v1 "k8s.io/api/core/v1"
)

var _ = Describe("Kube2e: Knative-Ingress with manual TLS enabled", func() {

	BeforeEach(func() {
		addTLSSecret()
		deployKnativeTestService(knativeTLSTestServiceFile())
	})

	AfterEach(func() {
		if err := deleteTLSSecret(); err != nil {
			log.Warnf("teardown failed, knative tls secret may still be present %v", err)
		}
		if err := deleteKnativeTestService(knativeTLSTestServiceFile()); err != nil {
			log.Warnf("teardown failed, knative test service may still be present %v", err)
		}
	})

	It("works", func() {
		ingressProxy := "knative-external-proxy"
		ingressPort := 443
		testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
			Protocol:          "https",
			Path:              "/",
			Method:            "GET",
			Host:              "helloworld-go.default.example.com",
			Service:           ingressProxy,
			Port:              ingressPort,
			ConnectionTimeout: 1,
			Verbose:           true,
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
	Expect(err).To(BeNil(), "it should generate the cert")
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
		CaBundle:           certs.CaCertificate,
	}

	err = kube.CreateTlsSecret(context.Background(), kubeClient, secretConfig)
	Expect(err).To(BeNil(), "it should create the tls secret")
}

func deleteTLSSecret() error {
	kubectlArgs := strings.Split("delete secret my-knative-tls-secret", " ")
	err := exec.RunCommandInput("kubectl", testHelper.RootDir, true, kubectlArgs...)
	if err != nil {
		return err
	}
	return nil
}

func knativeTLSTestServiceFile() string {
	return filepath.Join(testHelper.RootDir, "test", "kube2e", "knative", "artifacts", "knative-hello-service-tls.yaml")
}
