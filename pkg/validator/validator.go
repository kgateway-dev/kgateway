package validator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

var (
	defaultEnvoyPath = "/usr/local/bin/envoy"
	// TODO(tim): avoid hardcoding the envoy image version in multiple places.
	defaultEnvoyImage = "quay.io/solo-io/envoy-gloo:1.35.2-patch1"
)

// ErrInvalidXDS is returned when Envoy rejects the supplied YAML.
var ErrInvalidXDS = errors.New("invalid xds configuration")

// Validator validates an Envoy bootstrap/partial YAML.
type Validator interface {
	// Validate validates the given YAML configuration. Returns an error
	// if the configuration is invalid.
	Validate(context.Context, string) error
}

// binaryValidator validates envoy using the binary.
type binaryValidator struct {
	path string
}

var _ Validator = &binaryValidator{}

// NewBinary creates a new binary validator. If path is empty, the default path is used.
func NewBinary(path ...string) Validator {
	if len(path) == 0 {
		path = []string{defaultEnvoyPath}
	}
	return &binaryValidator{path: path[0]}
}

func (b *binaryValidator) Validate(ctx context.Context, yaml string) error {
	cmd := exec.CommandContext(ctx, b.path, "--mode", "validate", "--config-yaml", yaml, "-l", "critical", "--log-format", "%v")
	cmd.Stdin = strings.NewReader(yaml)
	var e bytes.Buffer
	cmd.Stderr = &e
	if err := cmd.Run(); err != nil {
		rawErr := strings.TrimSpace(e.String())
		if _, ok := err.(*exec.ExitError); ok {
			if rawErr == "" {
				rawErr = err.Error()
			}
			return fmt.Errorf("%w: %s", ErrInvalidXDS, rawErr)
		}
		return fmt.Errorf("envoy validate invocation failed: %v", err)
	}
	return nil
}

type dockerValidator struct {
	img string
}

var _ Validator = &dockerValidator{}

// NewDocker creates a new docker validator. If img is empty, the default image is used.
func NewDocker(img ...string) Validator {
	if len(img) == 0 {
		img = []string{defaultEnvoyImage}
	}
	return &dockerValidator{img: img[0]}
}

func (d *dockerValidator) Validate(ctx context.Context, yaml string) error {
	cmd := exec.CommandContext(
		ctx,
		"docker", "run",
		"--rm",
		"-i",
		"--platform", "linux/amd64",
		d.img,
		"--mode",
		"validate",
		"--config-yaml", yaml,
		"-l", "critical",
		"--log-format", "%v",
	)
	cmd.Stdin = strings.NewReader(yaml)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return nil
	}

	rawErr := strings.TrimSpace(stderr.String())
	if _, ok := err.(*exec.ExitError); ok {
		// Extract just the envoy error message, ignoring Docker pull output
		if envoyErr := extractEnvoyError(rawErr); envoyErr != "" {
			return fmt.Errorf("%w: %s", ErrInvalidXDS, envoyErr)
		}
		if rawErr == "" {
			rawErr = err.Error()
		}
		return fmt.Errorf("%w: %s", ErrInvalidXDS, rawErr)
	}
	return fmt.Errorf("envoy validate invocation failed: %v", err)
}

// extractEnvoyError extracts the actual Envoy validation error from stderr output,
// ignoring Docker pull progress and other noise that comes before the error.
func extractEnvoyError(stderr string) string {
	lines := strings.Split(stderr, "\n")
	// find the first line containing the Envoy error message. see:
	// https://github.com/envoyproxy/envoy/blob/d552b66f5d70ddd9e13c68c40f70729a45fb24e0/source/server/config_validation/server.cc#L75
	errorIndex := slices.IndexFunc(lines, func(line string) bool {
		return strings.Contains(strings.TrimSpace(line), "error initializing configuration")
	})
	if errorIndex == -1 {
		return ""
	}
	// extract all remaining lines that are relevant error context
	remainingLines := make([]string, 0, len(lines)-errorIndex)
	for i := errorIndex; i < len(lines); i++ {
		if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
			remainingLines = append(remainingLines, trimmed)
		}
	}
	return strings.Join(remainingLines, " ")
}
