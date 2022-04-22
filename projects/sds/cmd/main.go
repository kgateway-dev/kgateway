package main

import (
	"context"
	"fmt"
	"os"

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
	// The NodeID of the envoy server reading from this SDS
	sdsClientDefault = "sds_client"
)

type Config struct {
	SdsServerAddress string `split_words:"true" default:"0.0.0.0:8234"` //sds_config target_uri in the envoy instance that it provides secrets to
	SdsClient        string `split_words:"true"`

	PodName      string `split_words:"true"`
	PodNamespace string `split_words:"true"`

	GlooMtlsSdsEnabled    bool   `split_words:"true"`
	GlooMtlsSecretDir     string `split_words:"true" default:"/etc/envoy/ssl/"`
	GlooServerCert        string `split_words:"true" default:"server_cert"`
	GlooValidationContext string `split_words:"true" default:"validation_context"`

	IstioMtlsSdsEnabled    bool   `split_words:"true"`
	IstioCertDir           string `split_words:"true" default:"/etc/istio-certs/"`
	IstioServerCert        string `split_words:"true" default:"istio_server_cert"`
	IstioValidationContext string `split_words:"true" default:"istio_validation_context"`
}

func main() {
	ctx := contextutils.WithLogger(context.Background(), "sds_server")
	ctx = contextutils.WithLoggerValues(ctx, "version", version.Version)

	contextutils.LoggerFrom(ctx).Info("initializing config")

	var c = setup(ctx)

	contextutils.LoggerFrom(ctx).Infow(
		"config loaded",
		zap.Bool("glooMtlsSdsEnabled", c.GlooMtlsSdsEnabled),
		zap.Bool("istioMtlsSdsEnabled", c.IstioMtlsSdsEnabled),
	)

	secrets := []server.Secret{}
	if c.IstioMtlsSdsEnabled {
		istioCertsSecret := server.Secret{
			ServerCert:        c.IstioServerCert,
			ValidationContext: c.IstioValidationContext,
			SslCaFile:         c.IstioCertDir + "root-cert.pem",
			SslCertFile:       c.IstioCertDir + "cert-chain.pem",
			SslKeyFile:        c.IstioCertDir + "key.pem",
		}
		secrets = append(secrets, istioCertsSecret)
	}

	if c.GlooMtlsSdsEnabled {
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

func setup(ctx context.Context) Config {
	var c Config
	err := envconfig.Process("", &c)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatal(err)
	}

	// Use default node ID from env vars if SDS_CLIENT not explicitly set.
	if c.SdsClient == "" {
		c.SdsClient = determineSdsClient(c)
	}

	// At least one must be enabled, otherwise we have nothing to do.
	if !c.GlooMtlsSdsEnabled && !c.IstioMtlsSdsEnabled {
		err := fmt.Errorf("at least one of Istio Cert rotation or Gloo Cert rotation must be enabled, using env vars GLOO_MTLS_SDS_ENABLED or ISTIO_MTLS_SDS_ENABLED")
		contextutils.LoggerFrom(ctx).Fatal(err)
	}
	return c
}

// determineSdsClient checks POD_NAME or POD_NAMESPACE
// environment vars to try and figure out the NodeID,
// otherwise returns the default "sds_client"
func determineSdsClient(c Config) string {
	if c.PodName != "" && c.PodNamespace != "" {
		return c.PodName + "." + c.PodNamespace
	}
	return sdsClientDefault
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

// fileExists checks to see if a file exists
func fileExists(filePath string) bool {
	err := retry.Do(
		func() error {
			_, err := os.Stat(filePath)
			return err
		},
		retry.Attempts(8), // Exponential backoff over ~13s
	)
	if err != nil {
		return false
	}
	return true
}
