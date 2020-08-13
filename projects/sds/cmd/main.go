package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/sds/pkg/run"
	"github.com/solo-io/gloo/projects/sds/pkg/server"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/avast/retry-go"
	"github.com/kelseyhightower/envconfig"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
)

var (
	// The Node ID.
	sdsClientDefault = os.Getenv("GATEWAY_PROXY_POD_NAME") + "." + os.Getenv("GATEWAY_PROXY_POD_NAMESPACE")
)

type Config struct {
	SdsServerAddress string `split_words:"true" default:"127.0.0.1:8234"` //sds_config target_uri in the envoy instance that it provides secrets to
	SdsClient        string `split_words:"true"`

	GlooRotationEnabled   bool   `split_words:"true"`
	GlooMtlsSecretDir     string `split_words:"true" default:"/etc/envoy/ssl/"`
	GlooServerCert        string `split_words:"true" default:"server_cert"`
	GlooValidationContext string `split_words:"true" default:"validation_context"`

	IstioRotationEnabled   bool   `split_words:"true"`
	IstioCertDir           string `split_words:"true" default:"/etc/istio-certs/"`
	IstioServerCert        string `split_words:"true" default:"istio_server_cert"`
	IstioValidationContext string `split_words:"true" default:"istio_validation_context"`
}

func main() {
	ctx := contextutils.WithLogger(context.Background(), "sds_server")
	ctx = contextutils.WithLoggerValues(ctx, "version", version.Version)

	contextutils.LoggerFrom(ctx).Info("initializing config")

	var c = setup()

	contextutils.LoggerFrom(ctx).Infow(
		"config loaded",
		zap.Bool("glooCertRotationEnabled", c.GlooRotationEnabled),
		zap.Bool("istioCertRotationEnabled", c.IstioRotationEnabled),
	)

	secrets := []server.Secret{}
	if c.IstioRotationEnabled {
		istioCertsSecret := server.Secret{
			ServerCert:        c.IstioServerCert,
			ValidationContext: c.IstioValidationContext,
			SslCaFile:         c.IstioCertDir + "root-cert.pem",
			SslCertFile:       c.IstioCertDir + "cert-chain.pem",
			SslKeyFile:        c.IstioCertDir + "key.pem",
		}
		secrets = append(secrets, istioCertsSecret)
	}

	if c.GlooRotationEnabled {
		glooMtlsSecret := server.Secret{
			ServerCert:        c.GlooServerCert,
			ValidationContext: c.GlooValidationContext,
			SslCaFile:         c.GlooMtlsSecretDir + v1.ServiceAccountRootCAKey,
			SslCertFile:       c.GlooMtlsSecretDir + v1.TLSCertKey,
			SslKeyFile:        c.GlooMtlsSecretDir + v1.TLSPrivateKeyKey,
		}
		secrets = append(secrets, glooMtlsSecret)
	}

	contextutils.LoggerFrom(ctx).Info("checking for existence of secrets")

	for _, s := range secrets {
		// Check to see if files exist first to avoid crashloops
		if err := checkFilesExist([]string{s.SslKeyFile, s.SslCertFile, s.SslCaFile}); err != nil {
			contextutils.LoggerFrom(ctx).Fatal(err)
		}
	}

	contextutils.LoggerFrom(ctx).Info("secrets confirmed present, proceeding to start SDS server")

	if err := run.Run(ctx, secrets, c.SdsClient, c.SdsServerAddress); err != nil {
		contextutils.LoggerFrom(ctx).Fatal(err)
	}
}

func setup() Config {
	var c Config
	err := envconfig.Process("cr", &c)
	if err != nil {
		panic("error, could not process sds config")
	}

	// Use podname.podnamepsace if sdsClient not explicitly set
	if c.SdsClient == "" {
		c.SdsClient = sdsClientDefault
	}

	// At least one must be enabled, otherwise we have nothing to do.
	if !c.GlooRotationEnabled && !c.IstioRotationEnabled {
		panic("at least one of Istio Cert rotation or Gloo Cert rotation must be enabled, using env vars CR_GLOO_ROTATION_ENABLED or CR_ISTIO_ROTATION_ENABLED")
	}
	return c
}

// checkFilesExist returns an err if any of the
// given filePaths do not exist.
func checkFilesExist(filePaths []string) error {
	for _, filePath := range filePaths {
		if !fileExists(filePath) {
			return fmt.Errorf("could not find file '%v'", filePath)
		}
	}
	return nil
}

// fileExists checks to see if a file exists,
// retrying for up to 10 seconds.
func fileExists(filePath string) bool {
	err := retry.Do(
		func() error {
			_, err := os.Stat(filePath)
			return err
		},
		// Retry for up to 10s
		retry.Delay(400*time.Millisecond),
		retry.Attempts(25),
	)
	if err != nil {
		return false
	}
	return true
}
