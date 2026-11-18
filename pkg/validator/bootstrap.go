package validator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	envoybootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/pkg/xds/bootstrap"
)

// ErrInvalidBootstrap is returned when Envoy rejects a proxy bootstrap, or crashes on
// it.
var ErrInvalidBootstrap = errors.New("Envoy rejected the bootstrap") //nolint:staticcheck // ST1005: the message starts with a proper noun

// ErrBootstrapValidatorUnavailable is returned when the controller cannot run Envoy
// to validate any proxy bootstrap until its Deployment or settings change: the Envoy
// binary, the docker CLI or the Envoy image's entrypoint is missing or cannot be run,
// the controller cannot write its temporary files, or the configured validator cannot
// validate bootstraps.
var ErrBootstrapValidatorUnavailable = errors.New("the controller cannot run Envoy to validate the bootstrap")

const (
	// validationNodeID is the node id the validated copy of a proxy bootstrap gets. The
	// proxy gets its own from --service-node.
	validationNodeID = "bootstrap-validation"
	// binaryValidationTimeout bounds one run of the Envoy binary. Envoy validates a
	// bootstrap in well under a second; the rest is room for a busy controller.
	binaryValidationTimeout = 30 * time.Second
	// dockerValidationTimeout bounds one run of Envoy in docker, which can include
	// pulling the image.
	dockerValidationTimeout = 5 * time.Minute
)

// removedGuardRe matches the message Envoy logs for a runtime guard it no longer
// knows, in any log format.
var removedGuardRe = regexp.MustCompile(`Using a removed guard (\S+?)\. In future version`)

// BootstrapValidator validates the bootstrap a managed proxy starts with.
type BootstrapValidator interface {
	// ValidateBootstrap runs bootstrap through envoy --mode validate, prepared to
	// match what the proxy pod runs. It returns an error wrapping:
	//   - ErrInvalidBootstrap when Envoy rejects or crashes on the bootstrap;
	//   - ErrBootstrapValidatorUnavailable when the controller cannot run Envoy for
	//     any bootstrap until its Deployment or settings change;
	//   - ctx.Err() when ctx ends first.
	// Any other error means Envoy did not finish, for a reason that can pass, such as
	// a full disk, the timeout, or Envoy being killed for memory; validating again can
	// succeed.
	ValidateBootstrap(ctx context.Context, bootstrap *envoybootstrapv3.Bootstrap) (BootstrapResult, error)
}

// BootstrapResult reports what Envoy logged about a bootstrap it accepted.
type BootstrapResult struct {
	// RemovedRuntimeGuards lists the runtime keys Envoy reported as removed guards,
	// sorted. Envoy accepts them and ignores their values. Envoy rate-limits the
	// message, so when the bootstrap sets more than two, some may be missing.
	RemovedRuntimeGuards []string
}

var (
	_ BootstrapValidator = &binaryValidator{}
	_ BootstrapValidator = &dockerValidator{}
)

// ValidateBootstrap validates bootstrap with the Envoy binary.
func (b *binaryValidator) ValidateBootstrap(ctx context.Context, bs *envoybootstrapv3.Bootstrap) (BootstrapResult, error) {
	return validateProxyBootstrap(ctx, bs, binaryValidationTimeout, func(ctx context.Context, _ string, args []string, config []byte) (string, error) {
		cmd := exec.CommandContext(ctx, b.path, args...) //nolint:gosec // G204: envoy binary with controlled args for config validation
		cmd.Env = append(cmd.Env, "ENVOY_DYNAMIC_MODULES_SEARCH_PATH=/usr/local/lib")
		return runEnvoy(cmd, config)
	})
}

// ValidateBootstrap validates bootstrap with Envoy in the validator's image, mounting
// the placeholder files at the same path in the container.
func (d *dockerValidator) ValidateBootstrap(ctx context.Context, bs *envoybootstrapv3.Bootstrap) (BootstrapResult, error) {
	return validateProxyBootstrap(ctx, bs, dockerValidationTimeout, func(ctx context.Context, dir string, args []string, config []byte) (string, error) {
		// Run Envoy in the image, with the placeholder files mounted.
		dockerArgs := []string{"run", "--rm", "-i"}
		if d.pull != "" {
			dockerArgs = append(dockerArgs, "--pull", d.pull)
		}
		dockerArgs = append(dockerArgs, "-v", dir+":"+dir+":ro", "--entrypoint", "/usr/local/bin/envoy", d.img)
		cmd := exec.CommandContext(ctx, "docker", append(dockerArgs, args...)...) //nolint:gosec // G204: docker command with controlled args for config validation
		stderr, err := runEnvoy(cmd, config)

		// Exit codes 125 to 127 come from docker, not Envoy: 125 when docker cannot run
		// the container, for example when the daemon is unreachable or the pull fails,
		// and 126 or 127 when the image's Envoy cannot be run or is missing.
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			msg := strings.TrimSpace(stderr)
			switch exitErr.ExitCode() {
			case 125:
				return stderr, fmt.Errorf("docker could not run the Envoy container: %s", msg)
			case 126, 127:
				return stderr, fmt.Errorf("%w: the image %s cannot run /usr/local/bin/envoy: %s", ErrBootstrapValidatorUnavailable, d.img, msg)
			}
		}
		return stderr, err
	})
}

// runEnvoy runs cmd with config on stdin and returns its stderr.
func runEnvoy(cmd *exec.Cmd, config []byte) (string, error) {
	var stderr bytes.Buffer
	cmd.Stdin = bytes.NewReader(config)
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
}

// envoyRunner runs Envoy with args and config on stdin, and returns its stderr. dir
// holds the placeholder files the config refers to.
type envoyRunner func(ctx context.Context, dir string, args []string, config []byte) (string, error)

// validateProxyBootstrap prepares a copy of bs to match what the proxy pod runs,
// validates it with run within timeout, and interprets Envoy's output.
func validateProxyBootstrap(ctx context.Context, bs *envoybootstrapv3.Bootstrap, timeout time.Duration, run envoyRunner) (BootstrapResult, error) {
	// Prepare the copy, with its placeholder files in a temporary directory.
	dir, err := os.MkdirTemp("", "kgateway-bootstrap-validation-")
	if err != nil {
		return BootstrapResult{}, fileError("failed to create a directory for bootstrap validation", err)
	}
	defer os.RemoveAll(dir)
	config, err := prepareProxyBootstrap(bs, dir)
	if err != nil {
		return BootstrapResult{}, err
	}

	// Run Envoy at error level, which includes removed runtime guards. The proxy's
	// own log format applies when the bootstrap sets one, since Envoy refuses it
	// together with --log-format.
	args := []string{"--mode", "validate", "--config-path", "/dev/fd/0", "-l", "error"}
	if bs.GetApplicationLogConfig().GetLogFormat() == nil {
		args = append(args, "--log-format", "%v")
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stderr, err := run(runCtx, dir, args, config)
	if err == nil {
		return BootstrapResult{RemovedRuntimeGuards: removedRuntimeGuards(stderr)}, nil
	}
	return BootstrapResult{}, classifyRunError(ctx, runCtx, timeout, stderr, err)
}

// classifyRunError returns the error to report for a run of Envoy that failed with
// err. ctx is the caller's context and runCtx the run's, which adds the timeout.
// Context checks come first, because the process killed when a context ends exits
// with SIGKILL.
func classifyRunError(ctx, runCtx context.Context, timeout time.Duration, stderr string, err error) error {
	// The caller stopped waiting, or the run took too long.
	if ctx.Err() != nil {
		return fmt.Errorf("bootstrap validation stopped: %w", ctx.Err())
	}
	if runCtx.Err() != nil {
		return fmt.Errorf("Envoy did not finish validating the bootstrap within %s", timeout) //nolint:staticcheck // ST1005: the message starts with a proper noun
	}

	// Envoy did not start, or docker failed before running it.
	exitErr, exited := errors.AsType[*exec.ExitError](err)
	if !exited {
		switch {
		case errors.Is(err, ErrBootstrapValidatorUnavailable):
			return err
		case errors.Is(err, exec.ErrNotFound), errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrPermission):
			return fmt.Errorf("%w: %w", ErrBootstrapValidatorUnavailable, err)
		}
		return fmt.Errorf("failed to run Envoy to validate the bootstrap: %w", err)
	}

	// Envoy exited on its own, with its reason in stderr.
	msg := extractEnvoyError(stderr)
	if msg == "" {
		msg = stderr
	}
	msg = normalizeEnvoyError(msg)

	// SIGKILL comes from outside Envoy, most likely the out-of-memory killer, so the
	// bootstrap may be fine. Another signal is a crash, which the proxy would hit too.
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		if status.Signal() == syscall.SIGKILL {
			return errors.New("Envoy was killed while validating the bootstrap, possibly because the controller ran out of memory") //nolint:staticcheck // ST1005: the message starts with a proper noun
		}
		if msg == "" {
			return fmt.Errorf("%w: Envoy crashed with signal %q", ErrInvalidBootstrap, status.Signal())
		}
		return fmt.Errorf("%w: Envoy crashed with signal %q: %s", ErrInvalidBootstrap, status.Signal(), msg)
	}

	// Otherwise Envoy rejected the bootstrap.
	if msg == "" {
		msg = exitErr.Error()
	}
	return fmt.Errorf("%w: %s", ErrInvalidBootstrap, msg)
}

// fileError wraps err from writing the validator's temporary files. A directory the
// controller may not write to fails the same way every time; another failure, such as
// a full disk, can pass.
func fileError(msg string, err error) error {
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.EROFS) {
		return fmt.Errorf("%w: %s: %w", ErrBootstrapValidatorUnavailable, msg, err)
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// prepareProxyBootstrap returns a copy of bs, as proto JSON, with the changes that
// let envoy --mode validate check it as the proxy pod runs it:
//   - a placeholder node id, which the proxy gets from --service-node;
//   - the SYSTEM_CA_CERT placeholder secret, which envoyinit adds in the pod;
//   - each path_config_source path pointed at an empty file in dir, because Envoy
//     requires the file to exist but does not read it in validate mode.
func prepareProxyBootstrap(bs *envoybootstrapv3.Bootstrap, dir string) ([]byte, error) {
	// Set the node id and the system CA secret.
	clone := proto.CloneOf(bs)
	if clone.Node == nil {
		clone.Node = &envoycorev3.Node{}
	}
	clone.Node.Id = validationNodeID
	if clone.StaticResources == nil {
		clone.StaticResources = &envoybootstrapv3.Bootstrap_StaticResources{}
	}
	clone.StaticResources.Secrets = append(clone.StaticResources.Secrets, bootstrap.SystemCAValidationSecretPlaceholder())

	// Point each file-based config source at an empty placeholder file.
	placeholders := map[string]string{}
	err := rewriteConfigSourcePaths(clone.ProtoReflect(), func(path string) (string, error) {
		if p, ok := placeholders[path]; ok {
			return p, nil
		}
		p := filepath.Join(dir, strconv.Itoa(len(placeholders))+filepath.Ext(path))
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			return "", fileError("failed to write a placeholder for "+path, err)
		}
		placeholders[path] = p
		return p, nil
	})
	if err != nil {
		return nil, err
	}

	return protojson.Marshal(clone)
}

// rewriteConfigSourcePaths sets each file path of a ConfigSource in m to what rewrite
// returns for it, including in messages packed in an Any.
func rewriteConfigSourcePaths(m protoreflect.Message, rewrite func(string) (string, error)) error {
	// Rewrite this message's path when it is a file-based config source.
	switch msg := m.Interface().(type) {
	case *envoycorev3.ConfigSource:
		return rewriteConfigSource(msg, rewrite)
	case *anypb.Any:
		return rewriteAny(msg, rewrite)
	}

	// Otherwise look in each message field, list and map.
	var err error
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList() && fd.Message() != nil:
			list := v.List()
			for i := 0; i < list.Len() && err == nil; i++ {
				err = rewriteConfigSourcePaths(list.Get(i).Message(), rewrite)
			}
		case fd.IsMap() && fd.MapValue().Message() != nil:
			v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
				err = rewriteConfigSourcePaths(mv.Message(), rewrite)
				return err == nil
			})
		case !fd.IsList() && !fd.IsMap() && fd.Message() != nil:
			err = rewriteConfigSourcePaths(v.Message(), rewrite)
		}
		return err == nil
	})
	return err
}

// rewriteConfigSource rewrites the file path of cs, if it has one.
func rewriteConfigSource(cs *envoycorev3.ConfigSource, rewrite func(string) (string, error)) error {
	var err error
	switch spec := cs.GetConfigSourceSpecifier().(type) {
	case *envoycorev3.ConfigSource_PathConfigSource:
		if spec.PathConfigSource.GetPath() != "" {
			spec.PathConfigSource.Path, err = rewrite(spec.PathConfigSource.GetPath())
		}
	case *envoycorev3.ConfigSource_Path:
		// The deprecated path field still configures a file source.
		if spec.Path != "" { //nolint:staticcheck // SA1019: see above
			spec.Path, err = rewrite(spec.Path) //nolint:staticcheck // SA1019: see above
		}
	}
	return err
}

// rewriteAny rewrites the config sources in the message packed in a, and packs it
// again.
func rewriteAny(a *anypb.Any, rewrite func(string) (string, error)) error {
	// Unpack the message. Parsing the bootstrap already resolved its type.
	inner, err := a.UnmarshalNew()
	if err != nil {
		return fmt.Errorf("failed to unpack %s: %w", a.GetTypeUrl(), err)
	}
	if err := rewriteConfigSourcePaths(inner.ProtoReflect(), rewrite); err != nil {
		return err
	}

	// Pack it again in place.
	a.Value, err = proto.MarshalOptions{Deterministic: true}.Marshal(inner)
	return err
}

// removedRuntimeGuards returns the runtime keys Envoy reported as removed guards in
// its log, sorted and without duplicates.
func removedRuntimeGuards(log string) []string {
	var keys []string
	for _, m := range removedGuardRe.FindAllStringSubmatch(log, -1) {
		keys = append(keys, m[1])
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}
