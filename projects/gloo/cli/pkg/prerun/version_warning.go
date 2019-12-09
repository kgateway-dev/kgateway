package prerun

import (
	"fmt"
	"os"

	linkedversion "github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	versioncmd "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	versiondiscovery "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/sets"

	"strings"
)

const (
	ContainerNameToCheck = "discovery"
)

func VersionMismatchWarning(opts *options.Options, cmd *cobra.Command) error {
	return WarnOnMismatch(os.Args[0], versioncmd.NewKube(opts.Metadata.Namespace), &defaultLogger{})
}

// use this logger interface, so that in the unit test we can accumulate lines that were output
type Logger interface {
	Printf(string, ...interface{})
	Println(string)
}

type defaultLogger struct {
}

func (d *defaultLogger) Printf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func (d *defaultLogger) Println(str string) {
	fmt.Fprintln(os.Stderr, str)
}

// visible for testing
func WarnOnMismatch(binaryName string, sv versioncmd.ServerVersion, logger Logger) error {
	clientServerVersions, err := versioncmd.GetClientServerVersions(sv)
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

	message := fmt.Sprintf("glooctl binary version (%s) differs from server components (%v) by at least a minor version.\n", linkedversion.Version, versions.List())

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
	logger.Println("Warning: Could not determine gloo client/server versions (is Gloo running outside of kubernetes?): " + err.Error())
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
			return nil, errors.Errorf("Unhandled server version type: %v", podVersion.VersionType)
		}
	}

	return versions, nil
}
