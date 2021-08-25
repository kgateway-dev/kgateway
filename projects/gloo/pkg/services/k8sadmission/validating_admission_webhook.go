package k8sadmission

import (
	"context"
	"net/http"

	"github.com/solo-io/gloo/pkg/utils/webhookutils"
	"github.com/solo-io/go-utils/contextutils"
)

const (
	ValidationPath = "/validation"
	validatorName  = "gloo"
)

type GlooWebhookConfig struct {
	Ctx            context.Context
	Port           int
	ServerCertPath string
	ServerKeyPath  string
}

func NewGlooValidatingWebhook(cfg *GlooWebhookConfig) (*http.Server, error) {
	handler := NewGlooValidationHandler(
		contextutils.WithLogger(cfg.Ctx, "gloo-validation-webhook"),
	)

	return webhookutils.NewWebhook(&webhookutils.WebhookConfig{
		Ctx:            cfg.Ctx,
		Port:           cfg.Port,
		ValidatorName:  validatorName,
		ValidationPath: ValidationPath,
		ServerCertPath: cfg.ServerCertPath,
		ServerKeyPath:  cfg.ServerKeyPath,
		Handler:        &handler,
	})
}
