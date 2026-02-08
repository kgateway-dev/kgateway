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
	// NOTE: We cannot use vanilla upstream image here because it won't have the rustformation dynamic
	//       modules bundled into the image and some strict validation test on transformation will not work.
	//       This can be a chicken and an egg problem if we need a fix in the rustformation module to
	//       fix the validation test. We will need to merge the fix PR first and wait for the image to
	//       be updated and then maybe update the golden files
	//       Also probably need to change this version when backporting or creating a new release
	defaultEnvoyImage = "ghcr.io/kgateway-dev/envoy-wrapper:v2.3.0-main"
)

// ErrInvalidXDS is returned when Envoy rejects the supplied JSON.
var ErrInvalidXDS = errors.New("invalid xds configuration")

// Validator validates an Envoy bootstrap/partial JSON.
type Validator interface {
	// Validate validates the given JSON configuration. Returns an error
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

func (b *binaryValidator) Validate(ctx context.Context, json string) error {
	cmd := exec.CommandContext(ctx, b.path, "--mode", "validate", "--config-path", "/dev/fd/0", "-l", "critical", "--log-format", "%v") //nolint:gosec // G204: envoy binary with controlled args for config validation
	cmd.Env = append(cmd.Env, "ENVOY_DYNAMIC_MODULES_SEARCH_PATH=/usr/local/lib")
	cmd.Stdin = strings.NewReader(json)
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
	img      string
	etcEnvoy string
}

type DockerValidatorOptions func(*dockerValidator)

func Image(img string) func(*dockerValidator) {
	return func(d *dockerValidator) {
		d.img = img
	}
}

func EtcEnvoyVolume(etcEnvoy string) func(*dockerValidator) {
	return func(d *dockerValidator) {
		d.etcEnvoy = etcEnvoy
	}
}

var _ Validator = &dockerValidator{}

// NewDocker creates a new docker validator. If img is empty, the default image is used.
func NewDocker(opts ...DockerValidatorOptions) Validator {
	ret := &dockerValidator{
		img: defaultEnvoyImage,
	}

	for _, opt := range opts {
		opt(ret)
	}

	return ret
}

func (d *dockerValidator) args() []string {
	args := []string{
		"run",
		"--rm",
		"-i",
		"--pull", "always",
	}
	if d.etcEnvoy != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/etc/envoy/:ro", d.etcEnvoy))
	}
	args = append(args,
		"--entrypoint", "/usr/local/bin/envoy",
		d.img,
		"--mode",
		"validate",
		"--service-node", "dummy-node",
		"--config-path", "/dev/fd/0",
		"-l", "critical",
		"--log-format", "%v",
	)
	return args
}

func (d *dockerValidator) Validate(ctx context.Context, json string) error {
	cmd := exec.CommandContext( //nolint:gosec // G204: docker command with controlled args for config validation
		ctx,
		"docker", d.args()...)

	cmd.Stdin = strings.NewReader(json)
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
