package prerun

import (
	"fmt"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	version2 "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/sets"

	"strings"
)

func VersionMismatchWarning(opts *options.Options, cmd *cobra.Command) error {
	return WarnOnMismatch(os.Args[0], version.NewKube(opts.Metadata.Namespace), &defaultLogger{})
}

// use this logger interface, so that in the unit test we can accumulate lines that were output
type Logger interface {
	Printf(string, ...interface{})
	Println(string)
}

type defaultLogger struct {
}

func (d *defaultLogger) Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

func (d *defaultLogger) Println(str string) {
	fmt.Println(str)
}

// visible for testing
func WarnOnMismatch(binaryName string, sv version.ServerVersion, logger Logger) error {
	clientServerVersions, err := version.GetClientServerVersions(sv)
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

	containerMetadatas, err := buildContainerMetadata(clientServerVersions.Server)
	if err != nil {
		warnOnError(err, logger)
		return nil
	}

	var minorVersionMismatches []*ContainerVersion
	var majorVersionMismatches []*ContainerVersion
	for _, containerMetadata := range containerMetadatas {
		if containerMetadata.Version.Major == glooctlVersion.Major && containerMetadata.Version.Minor != glooctlVersion.Minor {
			minorVersionMismatches = append(minorVersionMismatches, containerMetadata)
		}
		if containerMetadata.Version.Major != glooctlVersion.Major {
			majorVersionMismatches = append(majorVersionMismatches, containerMetadata)
		}
	}

	if len(minorVersionMismatches) > 0 || len(majorVersionMismatches) > 0 {
		logger.Println("----------")
		if len(minorVersionMismatches) > 0 {
			logger.Println(BuildVersionMismatchMessage(minorVersionMismatches, glooctlVersionStr, "minor"))
		}
		if len(majorVersionMismatches) > 0 {
			logger.Println(BuildVersionMismatchMessage(majorVersionMismatches, glooctlVersionStr, "major"))
		}
		logger.Println("")
		logger.Println(BuildSuggestedUpgradeCommand(binaryName, append(minorVersionMismatches, majorVersionMismatches...)))
		logger.Println("----------\n")
	}

	return nil
}

// visible for testing
func BuildVersionMismatchMessage(mismatches []*ContainerVersion, glooctlVersion, mismatchKind string) string {
	var containersToReport []string
	for _, mismatch := range mismatches {
		containersToReport = append(containersToReport, fmt.Sprintf("%s@%s", mismatch.ContainerName, mismatch.Version.String()))
	}
	return fmt.Sprintf("WARNING: glooctl@%s has a different %s version than the following server containers: %s", glooctlVersion, mismatchKind, strings.Join(containersToReport, ", "))
}

// visible for testing
func BuildSuggestedUpgradeCommand(binaryName string, mismatches []*ContainerVersion) string {
	versions := sets.NewString()
	for _, mismatch := range mismatches {
		versions.Insert(mismatch.Version.String())
	}

	message := ""

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

func buildContainerMetadata(podVersions []*version2.ServerVersion) ([]*ContainerVersion, error) {
	var versions []*ContainerVersion
	for _, podVersion := range podVersions {
		switch podVersion.VersionType.(type) {
		case *version2.ServerVersion_Kubernetes:
			for _, container := range podVersion.GetKubernetes().Containers {
				containerVersion, err := versionutils.ParseVersion("v" + container.Tag)
				if err != nil {
					// if the container version doesn't match our versioning scheme
					// (ie, as of writing this the redis container is on version "5")
					// then just skip it
					continue
				}

				versions = append(versions, &ContainerVersion{
					ContainerName: container.Name,
					Version:       containerVersion,
				})
			}
		default:
			return nil, errors.Errorf("Unhandled server version type: %v", podVersion.VersionType)
		}
	}

	return versions, nil
}
