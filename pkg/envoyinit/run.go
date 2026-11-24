package envoyinit

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/downward"
	"github.com/kgateway-dev/kgateway/v2/internal/envoyinit/pkg/utils"
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

// RunEnvoy run Envoy with bootstrap configuration injected from a file
func RunEnvoy(envoyExecutable, inputPath, outputPath string) {
	// 1. Transform the configuration using the Kubernetes Downward API
	waitForInputConfig(inputPath, filepath.Dir(defaultInputConfigPath))
	bootstrapConfig, err := getAndTransformConfig(inputPath)
	if err != nil {
		// The "initializer failed" prefix is asserted by the envoy-wrapper
		// container-structure test (test/container-structure/envoy-wrapper.yaml)
		// as proof the binary loads and runs; update that test if this changes.
		log.Fatalf("initializer failed: %v", err)
	}

	caPath, err := getOSRootFilePath()
	if err != nil {
		log.Printf("Failed to get a supported OS CA certificate path: %v", err)
	}
	if caPath != "" {
		log.Printf("Using OS CA certificate for proxy: %s", caPath)
		withCA, err := addSystemCASecret([]byte(bootstrapConfig), caPath)
		if err != nil {
			log.Fatalf("failed to add the system CA secret to the bootstrap: %v", err)
		}
		bootstrapConfig = string(withCA)
	}

	// 2. Write to a file for debug purposes
	// since this operation is meant only for debug purposes, we ignore the error
	// this might fail if root fs is read only
	_ = os.WriteFile(outputPath, []byte(bootstrapConfig), 0o444) //nolint:gosec // G306: Debug file with read-only permissions is intentional

	// 3. Execute Envoy with the provided configuration
	args := []string{envoyExecutable, "--config-yaml", bootstrapConfig}
	if len(os.Args) > 1 {
		args = append(args, os.Args[1:]...)
	}
	if err = syscall.Exec(args[0], args, os.Environ()); err != nil { //nolint:gosec // G204: Executing Envoy with validated arguments is intentional
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

// addSystemCASecret appends a static secret named SYSTEM_CA_CERT, which trusts the CA
// bundle at caPath, to the bootstrap's static_resources.secrets, and returns the
// bootstrap as YAML.
//
// It edits the bootstrap as a generic document rather than an Envoy Bootstrap proto, so
// a field or extension type this binary does not know reaches Envoy with its values, and
// Envoy alone decides whether the bootstrap is valid. Re-encoding can change key order
// and number formatting, but not values: integers are not rounded through float64. A
// bootstrap may spell the field static_resources or staticResources; the secret goes
// into whichever is present, and into a new static_resources when neither is.
func addSystemCASecret(bootstrapYAML []byte, caPath string) ([]byte, error) {
	// Decode the bootstrap into a generic map without rounding integers.
	bootstrap, err := downward.DecodeYAMLMap(bootstrapYAML)
	if err != nil {
		return nil, fmt.Errorf("the bootstrap %w", err)
	}

	// Find static_resources under either spelling, creating it when neither is set.
	key := "static_resources"
	if _, ok := bootstrap[key]; !ok {
		if _, ok := bootstrap["staticResources"]; ok {
			key = "staticResources"
		}
	}
	staticResources, ok := bootstrap[key].(map[string]any)
	if !ok {
		if bootstrap[key] != nil {
			return nil, fmt.Errorf("the bootstrap's %s is not a mapping", key)
		}
		staticResources = map[string]any{}
		bootstrap[key] = staticResources
	}

	// Append the secret to the existing secrets.
	secrets, ok := staticResources["secrets"].([]any)
	if !ok && staticResources["secrets"] != nil {
		return nil, fmt.Errorf("the bootstrap's %s.secrets is not a list", key)
	}
	staticResources["secrets"] = append(secrets, map[string]any{
		"name": utils.SystemCaSecretName,
		"validation_context": map[string]any{
			"trusted_ca": map[string]any{
				"filename": caPath,
			},
		},
	})

	// Encode the result as YAML.
	out, err := yaml.Marshal(bootstrap)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the bootstrap: %w", err)
	}
	return out, nil
}

// inputConfigPollInterval is how often envoyinit checks for a missing bootstrap file
// that INPUT_CONF names.
const inputConfigPollInterval = time.Second

// waitForInputConfig blocks until path exists, but only when INPUT_CONF is set and path
// is inside configDir, where the generated bootstrap ConfigMap is mounted. Otherwise it
// returns at once, and a missing file fails the read that follows.
//
// The controller sets INPUT_CONF to the ConfigMap key that holds the pod's bootstrap
// revision. kubelet serves ConfigMap volumes from a watch-based cache, so a pod created
// right after the ConfigMap is updated can briefly see the previous content, which lacks
// that key. The wait has no time limit; the pod's startup probe bounds it.
func waitForInputConfig(path, configDir string) {
	// Only a key of the generated ConfigMap can be waited for.
	if os.Getenv(inputConfigPathEnv) == "" || !strings.HasPrefix(filepath.Clean(path), filepath.Clean(configDir)+string(filepath.Separator)) {
		return
	}

	waitForFile(path, inputConfigPollInterval, os.Stat, log.Printf)
}

// waitForFile returns once stat finds path, checking every interval while stat says it
// does not exist. Any other stat error returns at once, so reading the file reports it.
func waitForFile(path string, interval time.Duration, stat func(string) (fs.FileInfo, error), logf func(string, ...any)) {
	// Return at once when the file is there, or when stat fails for another reason.
	_, err := stat(path)
	if !errors.Is(err, fs.ErrNotExist) {
		return
	}

	// Poll until the file appears.
	logf("Waiting for the bootstrap file %s to appear in the mounted ConfigMap", path)
	start := time.Now()
	for errors.Is(err, fs.ErrNotExist) {
		time.Sleep(interval)
		_, err = stat(path)
	}
	logf("The bootstrap file %s appeared after %s", path, time.Since(start).Round(time.Millisecond))
}
