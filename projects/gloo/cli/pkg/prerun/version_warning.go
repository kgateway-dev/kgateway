package prerun

import (
	"context"
	"fmt"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"

	"github.com/spf13/cobra"

	"github.com/rotisserie/eris"
	linkedversion "github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	versioncmd "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	versiondiscovery "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/versionutils"
	"k8s.io/apimachinery/pkg/util/sets"

	"strings"
)

const (
	ContainerNameToCheck = "discovery"
)

func VersionMismatchWarning(opts *options.Options, cmd *cobra.Command) error {
	// Only Kubernetes provides client/server version information. Only check for a version
	// mismatch if Kubernetes is enabled (i.e. Consul is not enabled)
	if opts.Top.Consul.UseConsul {
		return nil
	}
	nsToCheck := opts.Metadata.Namespace
	// TODO: only use metadata namespace flag, install namespace can be populated from metadata namespace or refactored out of the opts
	if nsToCheck == flagutils.DefaultNamespace && opts.Install.Gloo.Namespace != flagutils.DefaultNamespace {
		nsToCheck = opts.Install.Gloo.Namespace
	}

	return WarnOnMismatch(opts.Top.Ctx, os.Args[0], versioncmd.NewKube(nsToCheck), &defaultLogger{})
}

// use this logger interface, so that in the unit test we can accumulate lines that were output
type Logger interface {
	Printf(string, ...interface{})
	Println(string)
}

type defaultLogger struct {
}

func (d *defaultLogger) Printf(format string, args ...interface{}) {
	// important that this remains writing to stderr, as we don't want this output to interfere with things like $(glooctl proxy url)
	fmt.Fprintf(os.Stderr, format, args...)
}

func (d *defaultLogger) Println(str string) {
	// important that this remains writing to stderr, as we don't want this output to interfere with things like $(glooctl proxy url)
	fmt.Fprintln(os.Stderr, str)
}

// visible for testing
func WarnOnMismatch(ctx context.Context, binaryName string, sv versioncmd.ServerVersion, logger Logger) error {
	clientServerVersions, err := versioncmd.GetClientServerVersions(ctx, sv)
	if err != nil {
		warnOnError(err, logger)
		return nil
	}

	glooctlVersionStr := "v" + clientServerVersions.Client.Version

	// two common cases I ran into in dev that we don't care about warning on
	if glooctlVersionStr == "vdev" || glooctlVersionStr == "vundefined" {
		return nil
	}

	glooctlVersion, err := versionutils.ParseVersion(glooctlVersionStr)
	if err != nil {
		warnOnError(err, logger)
		return nil
	}

	openSourceVersions, err := getOpenSourceVersions(clientServerVersions.Server)
	if err != nil {
		warnOnError(err, logger)
		return nil
	}

	var minorVersionMismatches []*versionutils.Version
	var majorVersionMismatches []*versionutils.Version
	for _, openSourceVersion := range openSourceVersions {
		if openSourceVersion.Major == glooctlVersion.Major && openSourceVersion.Minor != glooctlVersion.Minor {
			minorVersionMismatches = append(minorVersionMismatches, openSourceVersion)
		}
		if openSourceVersion.Major != glooctlVersion.Major {
			majorVersionMismatches = append(majorVersionMismatches, openSourceVersion)
		}
	}

	if len(minorVersionMismatches) > 0 || len(majorVersionMismatches) > 0 {
		logger.Println("----------")
		logger.Println(BuildSuggestedUpgradeCommand(binaryName, append(minorVersionMismatches, majorVersionMismatches...)))
		logger.Println("----------\n")
	}

	return nil
}

// visible for testing
func BuildSuggestedUpgradeCommand(binaryName string, mismatches []*versionutils.Version) string {
	versions := sets.NewString()
	for _, mismatch := range mismatches {
		versions.Insert(mismatch.String())
	}

	message := fmt.Sprintf("glooctl binary version (%s) differs from server components (%v) by at least a minor version.\n", linkedversion.Version, strings.Join(versions.List(), ","))

	if versions.Len() > 1 {
		message += "Multiple server versions found. Consider running any of:"
	} else {
		message += "Consider running:"
	}

	var suggestedCommands []string
	for _, v := range versions.UnsortedList() {
		suggestedCommands = append(suggestedCommands, fmt.Sprintf("%s upgrade --release=%s", binaryName, v))
	}

	return fmt.Sprintf("%s\n%s", message, strings.Join(suggestedCommands, "\n"))
}

// Gloo may not be running in a kubernetes environment, so don't error out the whole command
// if we couldn't find the version
func warnOnError(err error, logger Logger) {
	logger.Println("Warning: Could not determine gloo server versions (is Gloo running outside of kubernetes?): " + err.Error())
}

type ContainerVersion struct {
	ContainerName string
	Version       *versionutils.Version
}

// return an array of open source gloo versions found in the cluster
// this is determined by looking at all the versions of discovery that we can find
func getOpenSourceVersions(podVersions []*versiondiscovery.ServerVersion) (versions []*versionutils.Version, err error) {
	for _, podVersion := range podVersions {
		switch podVersion.VersionType.(type) {
		case *versiondiscovery.ServerVersion_Kubernetes:
			for _, container := range podVersion.GetKubernetes().Containers {
				containerVersion, err := versionutils.ParseVersion("v" + container.Tag)
				if err != nil {
					// if the container version doesn't match our versioning scheme
					// (ie, as of writing this the redis container is on version "5")
					// then just skip it
					continue
				}

				if container.Name == ContainerNameToCheck {
					versions = append(versions, containerVersion)
				}
			}
		default:
			return nil, eris.Errorf("Unhandled server version type: %v", podVersion.VersionType)
		}
	}

	return versions, nil
}
