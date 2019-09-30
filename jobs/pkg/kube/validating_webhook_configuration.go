package kube

import (
	"context"

	"github.com/pkg/errors"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"k8s.io/api/admissionregistration/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type WebhookTlsConfig struct {
	ServiceName, ServiceNamespace string
	CaBundle                      []byte
}

func UpdateValidatingWebhookConfigurationCaBundle(ctx context.Context, kube kubernetes.Interface, vwcName string, cfg WebhookTlsConfig) error {
	contextutils.LoggerFrom(ctx).Infow("attempting to patch caBundle for ValidatingWebhookConfiguration", zap.String("svc", cfg.ServiceName), zap.String("vwc", vwcName))

	vwc, err := kube.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(vwcName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve vwc")
	}

	setCaBundle(ctx, vwc, cfg)

	if _, err := kube.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Update(vwc); err != nil {
		return errors.Wrapf(err, "failed to update vwc")
	}

	return nil
}

func setCaBundle(ctx context.Context, vwc *v1beta1.ValidatingWebhookConfiguration, cfg WebhookTlsConfig) {

	encodedCaBundle := cfg.CaBundle

	for i, wh := range vwc.Webhooks {
		if wh.ClientConfig.Service == nil {
			continue
		}

		svcName, svcNamespace := wh.ClientConfig.Service.Name, wh.ClientConfig.Service.Namespace

		// if we find a webhook cfg that targets our service, update it
		if svcName == cfg.ServiceName && svcNamespace == cfg.ServiceNamespace {
			wh.ClientConfig.CABundle = encodedCaBundle

			vwc.Webhooks[i] = wh

			contextutils.LoggerFrom(ctx).Infow("set CA bundle on ValidatingWebhookConfiguration", zap.String("svc", svcName), zap.String("vwc", vwc.Name), zap.Int("webhook", i))
		}
	}
}
