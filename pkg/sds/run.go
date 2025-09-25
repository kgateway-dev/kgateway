package sds

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/avast/retry-go"
	"github.com/kelseyhightower/envconfig"

	"github.com/solo-io/go-utils/stats"

	"github.com/kgateway-dev/kgateway/v2/internal/sds/pkg/run"
	"github.com/kgateway-dev/kgateway/v2/internal/sds/pkg/server"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

var (
	// The NodeID of the envoy server reading from this SDS
	sdsClientDefault = "sds_client"
	sdsComponentName = "sds_server"
	logger           = logging.New(sdsComponentName)
)

type Config struct {
	SdsServerAddress string `split_words:"true" default:"0.0.0.0:8234"` // sds_config target_uri in the envoy instance that it provides secrets to
	SdsClient        string `split_words:"true"`

	PodName      string `split_words:"true"`
	PodNamespace string `split_words:"true"`

	IstioMtlsSdsEnabled    bool   `split_words:"true"`
	IstioCertDir           string `split_words:"true" default:"/etc/istio-certs/"`
	IstioServerCert        string `split_words:"true" default:"istio_server_cert"`
	IstioValidationContext string `split_words:"true" default:"istio_validation_context"`
}

func RunMain() {
	// Initialize stats server to dynamically change log level. This will also use LOG_LEVEL if set.
	stats.ConditionallyStartStatsServer()
	logger.Info("initializing config")

	c := setup()

	//nolint:sloglint // ignore key case
	logger.Info(
		"config loaded",
		slog.Bool("istioMtlsSdsEnabled", c.IstioMtlsSdsEnabled),
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

	logger.Info("checking for existence of secrets")

	for _, s := range secrets {
		// Check to see if files exist first to avoid crashloops
		if err := checkFilesExist([]string{s.SslKeyFile, s.SslCertFile, s.SslCaFile}); err != nil {
			log.Fatalf("secrets check failed: %v", err)
		}
	}

	logger.Info("secrets confirmed present, proceeding to start SDS server")

	if err := run.Run(context.Background(), secrets, c.SdsClient, c.SdsServerAddress, logger); err != nil {
		log.Fatalf("failed to run SDS server: %v", err)
	}
}

func setup() Config {
	var c Config
	err := envconfig.Process("", &c)
	if err != nil {
		log.Fatalf("failed to process env config: %v", err)
	}

	// Use default node ID from env vars if SDS_CLIENT not explicitly set.
	if c.SdsClient == "" {
		c.SdsClient = determineSdsClient(c)
	}

	if !c.IstioMtlsSdsEnabled {
		err := fmt.Errorf("Istio Cert rotation must be enabled, using env var ISTIO_MTLS_SDS_ENABLED")
		log.Fatalf("invalid config: %v", err)
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
