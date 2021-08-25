package webhookutils

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/solo-io/gloo/pkg/utils/certprovider"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type Webhook *http.Server

type WebhookConfig struct {
	Ctx            context.Context
	Port           int
	ValidatorName  string
	ValidationPath string
	ServerCertPath string
	ServerKeyPath  string
	Handler        *http.Handler
}

func NewWebhook(config *WebhookConfig) (Webhook, error) {
	addr := fmt.Sprintf(":%v", config.Port)

	certProvider, err := certprovider.NewCertificateProvider(
		config.ValidatorName,
		config.ServerCertPath,
		config.ServerKeyPath,
		log.New(&debugLogger{ctx: config.Ctx}, "validation-webhook-certificate-watcher", log.LstdFlags),
		config.Ctx,
		10*time.Second,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "loading TLS certificate provider")
	}

	tlsConfig := &tls.Config{GetCertificate: certProvider.GetCertificateFunc()}
	mux := http.NewServeMux()
	mux.Handle(config.ValidationPath, *config.Handler)
	errorLog := log.New(&debugLogger{ctx: config.Ctx}, "validation-webhook-server", log.LstdFlags)

	return &http.Server{
		Addr:      addr,
		TLSConfig: tlsConfig,
		Handler:   mux,
		ErrorLog:  errorLog,
	}, nil
}

type debugLogger struct{ ctx context.Context }

func (l *debugLogger) Write(p []byte) (n int, err error) {
	contextutils.LoggerFrom(l.ctx).Debug(string(p))
	return len(p), nil
}
