package envoyinit

import (
	"bytes"
	"context"
	"errors"
	"log"
	"log/slog"
	"os"
	"syscall"
	"time"

	envoy_config_bootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/rotisserie/eris"

	"github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/downward"
	"github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmdutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/protoutils"
)

const (
	// Environment variable for the file that is used to inject input configuration used to bootstrap envoy
	inputConfigPathEnv     = "INPUT_CONF"
	defaultInputConfigPath = "/etc/envoy/envoy.yaml"

	// Environment variable for the file that is written to with transformed bootstrap configuration
	outputConfigPathEnv     = "OUTPUT_CONF"
	defaultOutputConfigPath = "/tmp/envoy.yaml"

	// Environment variable for the path to the envoy executable
	envoyExecutableEnv     = "ENVOY"
	defaultEnvoyExecutable = "/usr/local/bin/envoy"
)

func RunEnvoyValidate(ctx context.Context, envoyExecutable, bootstrapConfig string) error {
	validateCmd := cmdutils.Command(ctx, envoyExecutable, "--mode", "validate", "--config-path", "/dev/fd/0",
		"-l", "critical", "--log-format", "%v")
	validateCmd = validateCmd.WithStdin(bytes.NewBufferString(bootstrapConfig))

	start := time.Now()
	err := validateCmd.Run()
	slog.Debug("envoy validation completed",
		"size_bytes", len(bootstrapConfig),
		"duration", time.Since(start))

	if err != nil {
		if os.IsNotExist(err) {
			// log a warning and return nil; will allow users to continue to run Gloo locally without
			// relying on the Gloo container with Envoy already published to the expected directory
			slog.Warn("unable to validate envoy configuration", "executable", envoyExecutable)
			return nil
		}
		return eris.Errorf("envoy validation mode output: %v, error: %v", err.OutputString(), err.Error())
	}

	return nil
}

// RunEnvoy run Envoy with bootstrap configuration injected from a file
func RunEnvoy(envoyExecutable, inputPath, outputPath string) {
	// 1. Transform the configuration using the Kubernetes Downward API
	bootstrapConfig, err := getAndTransformConfig(inputPath)
	if err != nil {
		log.Fatalf("initializer failed: %v", err)
	}

	caPath, err := getOSRootFilePath()
	if err != nil {
		log.Printf("Failed to get a supported OS CA certificate path: %v", err)
	}
	if caPath != "" {
		log.Printf("Using OS CA certificate for proxy: %s", caPath)
		//If the CA cert path is set, we need to set the CA cert path in the bootstrap config
		var bootstrap envoy_config_bootstrap.Bootstrap
		err := protoutils.UnmarshalYaml([]byte(bootstrapConfig), &bootstrap)
		if err != nil {
			log.Fatalf("failed to unmarshal bootstrap config: %v", err)
		}
		bootstrap.GetStaticResources().Secrets = append(bootstrap.GetStaticResources().GetSecrets(), &tlsv3.Secret{
			Name: utils.SystemCaSecretName,
			Type: &tlsv3.Secret_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{
					TrustedCa: &corev3.DataSource{
						Specifier: &corev3.DataSource_Filename{
							Filename: caPath,
						},
					},
				},
			},
		})

		newBootstrapConfig, err := protoutils.MarshalBytes(&bootstrap)
		if err != nil {
			log.Fatalf("failed to marshal bootstrap config: %v", err)
		}
		bootstrapConfig = string(newBootstrapConfig)
	}

	// 2. Write to a file for debug purposes
	// since this operation is meant only for debug purposes, we ignore the error
	// this might fail if root fs is read only
	_ = os.WriteFile(outputPath, []byte(bootstrapConfig), 0444)

	// 3. Execute Envoy with the provided configuration
	args := []string{envoyExecutable, "--config-yaml", bootstrapConfig}
	if len(os.Args) > 1 {
		args = append(args, os.Args[1:]...)
	}
	if err = syscall.Exec(args[0], args, os.Environ()); err != nil {
		panic(err)
	}
}

// GetInputConfigPath returns the path to a file containing the Envoy bootstrap configuration
// This configuration may leverage the Kubernetes Downward API
// https://kubernetes.io/docs/tasks/inject-data-application/downward-api-volume-expose-pod-information/#the-downward-api
func GetInputConfigPath() string {
	return getEnvOrDefault(inputConfigPathEnv, defaultInputConfigPath)
}

// GetOutputConfigPath returns the path to a file where the raw Envoy bootstrap configuration will
// be persisted for debugging purposes
func GetOutputConfigPath() string {
	return getEnvOrDefault(outputConfigPathEnv, defaultOutputConfigPath)
}

// GetEnvoyExecutable returns the Envoy executable
func GetEnvoyExecutable() string {
	return getEnvOrDefault(envoyExecutableEnv, defaultEnvoyExecutable)
}

// getEnvOrDefault returns the value of the environment variable, if one exists, or a default string
func getEnvOrDefault(envName, defaultValue string) string {
	maybeEnvValue := os.Getenv(envName)
	if maybeEnvValue != "" {
		return maybeEnvValue
	}
	return defaultValue
}

// getAndTransformConfig reads a file, transforms it using the Downward API
// and returns the transformed configuration
func getAndTransformConfig(inputFile string) (string, error) {
	inReader, err := os.Open(inputFile)
	if err != nil {
		return "", err
	}
	defer inReader.Close()

	var buffer bytes.Buffer
	err = downward.Transform(inReader, &buffer)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

// getOSRootFilePath returns the first file path detected from a list of known CA certificate file paths.
// If none of the known CA certificate files are found, a warning in printed and an empty string is returned.
// Based on https://github.com/istio/istio/blob/d43c77c71df0150fa904d74bf6520d9e37180a1c/pkg/security/security.go#L463
func getOSRootFilePath() (string, error) {
	// Get and store the OS CA certificate path for Linux systems
	// Source of CA File Paths: https://golang.org/src/crypto/x509/root_linux.go
	certFiles := []string{
		"/etc/ssl/certs/ca-certificates.crt",                // Debian/Ubuntu/Gentoo etc.
		"/etc/pki/tls/certs/ca-bundle.crt",                  // Fedora/RHEL 6
		"/etc/ssl/ca-bundle.pem",                            // OpenSUSE
		"/etc/pki/tls/cacert.pem",                           // OpenELEC
		"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", // CentOS/RHEL 7
		"/etc/ssl/cert.pem",                                 // Alpine Linux
		"/usr/local/etc/ssl/cert.pem",                       // FreeBSD
		"/etc/ssl/certs/ca-certificates",                    // Talos Linux
	}

	for _, cert := range certFiles {
		// Use the first file found
		if _, err := os.Stat(cert); err == nil {
			return cert, nil
		}
	}
	return "", errors.New("OS CA Cert could not be found for agent")
}
