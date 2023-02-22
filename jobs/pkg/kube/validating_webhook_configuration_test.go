package kube_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	. "github.com/solo-io/gloo/jobs/pkg/kube"
)

var _ = Describe("ValidatingWebhookConfiguration", func() {
	It("updates a vwc with the provided cacert", func() {

		ctx, cancel := context.WithCancel(context.Background())
		defer func() { cancel() }()

		kube := fake.NewSimpleClientset()
		vwcCfg := WebhookTlsConfig{
			ServiceName:      "mysvc",
			ServiceNamespace: "mynamespace",
			CaBundle:         []byte{1, 2, 3},
		}

		vwcName := "myvwc"

		expectedVwc, err := kube.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(ctx, &v1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: vwcName},
			Webhooks: []v1.ValidatingWebhook{
				{Name: "ignored"},
				{
					Name: "foo",
					ClientConfig: v1.WebhookClientConfig{
						Service: &v1.ServiceReference{
							Name:      vwcCfg.ServiceName,
							Namespace: vwcCfg.ServiceNamespace,
						},
					},
				},
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		expectedVwc.Webhooks[1].ClientConfig.CABundle = vwcCfg.CaBundle

		err = UpdateValidatingWebhookConfigurationCaBundle(context.TODO(), kube, vwcName, vwcCfg)
		Expect(err).NotTo(HaveOccurred())

		patchedVwc, err := kube.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, vwcName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(patchedVwc).To(Equal(expectedVwc))
	})
})
