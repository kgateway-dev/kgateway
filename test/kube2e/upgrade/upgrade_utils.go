package upgrade

import (
	"context"
	"fmt"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"io/ioutil"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/changelogutils"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/versionutils"
)

var (
	FirstReleaseError = "First Release of Minor"
)

// Type used to sort Versions
type ByVersion []*versionutils.Version

func (a ByVersion) Len() int { return len(a) }
func (a ByVersion) Less(i, j int) bool {
	var version1 = *a[i]
	var version2 = *a[j]
	return version2.MustIsGreaterThan(version1)
}
func (a ByVersion) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

func GetUpgradeVersions(ctx context.Context, repoName string) (lastMinorLatestPatchVersion *versionutils.Version, currentMinorLatestPatchVersion *versionutils.Version, err error) {
	currentMinorLatestPatchVersion, curMinorErr := GetLastReleaseOfCurrentMinor(repoName)
	if curMinorErr != nil {
		if curMinorErr.Error() != FirstReleaseError {
			return nil, nil, curMinorErr
		}
	}
	lastMinorLatestPatchVersion, lastMinorErr := GetLatestReleasedVersion(ctx, repoName, currentMinorLatestPatchVersion.Major, currentMinorLatestPatchVersion.Minor-1)
	if lastMinorErr != nil {
		return nil, nil, lastMinorErr
	}
	return lastMinorLatestPatchVersion, currentMinorLatestPatchVersion, curMinorErr
}

func GetLastReleaseOfCurrentMinor(repoName string) (*versionutils.Version, error) { // pull out to const
	_, filename, _, _ := runtime.Caller(0) //get info about what is calling the function
	fmt.Printf(filename)
	fParts := strings.Split(filename, string(os.PathSeparator))
	splitIdx := 0
	//we can end up in a situation where the path contains the repo_name twice when running in ci - keep going until we find the last use ex: /home/runner/work/gloo/gloo/test/kube2e/upgrade/junit.xml
	for idx, dir := range fParts {
		if dir == repoName {
			splitIdx = idx
		}
	}
	pathToChangelogs := filepath.Join(fParts[:splitIdx+1]...)
	pathToChangelogs = filepath.Join(pathToChangelogs, changelogutils.ChangelogDirectory)
	pathToChangelogs = string(os.PathSeparator) + pathToChangelogs

	files, err := os.ReadDir(pathToChangelogs)
	if err != nil {
		return nil, changelogutils.ReadChangelogDirError(err)
	}

	versions := make([]*versionutils.Version, len(files)-1) //ignore validation file
	for idx, f := range files {
		if f.Name() != "validation.yaml" {
			version, err := versionutils.ParseVersion(f.Name())
			if err != nil {
				return nil, errors.Errorf("Could not get version for changelog folder: %s\n", f.Name())
			}
			versions[idx] = version
		}
	}

	sort.Sort(ByVersion(versions))
	//first release of minor
	if versions[len(versions)-1].Minor != versions[len(versions)-2].Minor {
		return versions[len(versions)-1], errors.Errorf(FirstReleaseError)
	}
	return versions[len(versions)-2], nil
}

func GetLatestReleasedVersion(ctx context.Context, repoName string, majorVersion, minorVersion int) (*versionutils.Version, error) {
	client, err := githubutils.GetClient(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create github client")
	}
	versionPrefix := fmt.Sprintf("v%d.%d", majorVersion, minorVersion)

	// inexact version requested may be prerelease and not have assets
	// We do assume that within a minor version we use monotonically increasing patch numbers
	// We also assume that the first release that is not strict semver is technically the largest
	for i := 0; i < 5; i++ {
		// Get the next page of
		listOpts := github.ListOptions{Page: i, PerPage: 10} // max per request
		releases, _, err := client.Repositories.ListReleases(ctx, "solo-io", repoName, &listOpts)
		if err != nil {
			return nil, errors.Wrapf(err, "error listing releases")
		}

		for _, release := range releases {
			v, err := versionutils.ParseVersion(*release.Name)
			if err != nil {
				continue
			}

			// either a major-minor was specified something of the form v%d.%d
			// or are searching for latest stable and have found the most recent
			// experimental and are now searching for a conforming release
			if versionPrefix != "" {
				// take the first valid from this version
				// as we assume increasing ordering
				if strings.HasPrefix(v.String(), versionPrefix) {
					return v, nil
				}
				continue
			}
		}
	}
	return nil, errors.Errorf("Could not find a recent release with version prefix: %s", versionPrefix)
}

func InstallGloo(testHelper *helper.SoloTestHelper, fromRelease string, strictValidation bool) {
	valueOverrideFile, cleanupFunc := kube2e.GetHelmValuesOverrideFile()
	defer cleanupFunc()

	// construct helm args
	var args = []string{"install", testHelper.HelmChartName}

	RunAndCleanCommand("helm", "repo", "add", testHelper.HelmChartName,
		"https://storage.googleapis.com/solo-public-helm", "--force-update")
	args = append(args, "gloo/gloo",
		"--version", fromRelease)

	args = append(args, "-n", testHelper.InstallNamespace,
		"--create-namespace",
		"--values", valueOverrideFile)
	if strictValidation {
		args = append(args, StrictValidationArgs...)
	}

	fmt.Printf("running helm with args: %v\n", args)
	RunAndCleanCommand("helm", args...)

	// Check that everything is OK
	CheckGlooOssHealthy(testHelper)
}

func UninstallGloo(testHelper *helper.SoloTestHelper, ctx context.Context, cancel context.CancelFunc) {
	Expect(testHelper).ToNot(BeNil())
	err := testHelper.UninstallGloo()
	Expect(err).NotTo(HaveOccurred())
	_, err = kube2e.MustKubeClient().CoreV1().Namespaces().Get(ctx, testHelper.InstallNamespace, metav1.GetOptions{})
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
	cancel()
}

// CRDs are applied to a cluster when performing a `helm install` operation
// However, `helm upgrade` intentionally does not apply CRDs (https://helm.sh/docs/topics/charts/#limitations-on-crds)
// Before performing the upgrade, we must manually apply any CRDs that were introduced since v1.9.0
func Crds(crdDir string) {
	// apply crds from the release we're upgrading to
	fmt.Printf("Upgrade crds: kubectl apply -f %s\n", crdDir)
	RunAndCleanCommand("kubectl", "apply", "-f", crdDir)
	// allow some time for the new crds to take effect
	time.Sleep(time.Second * 10)
}

// upgrade the version of gloo to the branch version
func GlooToBranchVersion(testHelper *helper.SoloTestHelper, chartUri string, crdDir string, strictValidation bool, additionalArgs []string) {
	Crds(crdDir)

	valueOverrideFile, cleanupFunc := GetHelmUpgradeValuesOverrideFile()
	defer cleanupFunc()

	var args = []string{"upgrade", testHelper.HelmChartName, chartUri,
		"-n", testHelper.InstallNamespace,
		"--values", valueOverrideFile}
	if strictValidation {
		args = append(args, StrictValidationArgs...)
	}
	args = append(args, additionalArgs...)

	fmt.Printf("running helm with args: %v\n", args)
	RunAndCleanCommand("helm", args...)

	//Check that everything is OK
	CheckGlooOssHealthy(testHelper)
}

func GetHelmUpgradeValuesOverrideFile() (filename string, cleanup func()) {
	values, err := ioutil.TempFile("", "values-*.yaml")
	Expect(err).NotTo(HaveOccurred())

	_, err = values.Write([]byte(`
global:
  image:
    pullPolicy: IfNotPresent
  glooRbac:
    namespaced: true
    nameSuffix: e2e-test-rbac-suffix
settings:
  singleNamespace: true
  create: true
  replaceInvalidRoutes: true
gateway:
  persistProxySpec: true
gatewayProxies:
  gatewayProxy:
    healthyPanicThreshold: 0
    gatewaySettings:
      # the KEYVALUE action type was first available in v1.11.11 (within the v1.11.x branch); this is a sanity check to
      # ensure we can upgrade without errors from an older version to a version with these new fields (i.e. we can set
      # the new fields on the Gateway CR during the helm upgrade, and that it will pass validation)
      customHttpGateway:
        options:
          dlp:
            dlpRules:
            - actions:
              - actionType: KEYVALUE
                keyValueAction:
                  keyToMask: test
                  name: test
`))
	Expect(err).NotTo(HaveOccurred())

	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() { _ = os.Remove(values.Name()) }
}

var StrictValidationArgs = []string{
	"--set", "gateway.validation.failurePolicy=Fail",
	"--set", "gateway.validation.allowWarnings=false",
	"--set", "gateway.validation.alwaysAcceptResources=false",
}

func RunAndCleanCommand(name string, arg ...string) []byte {
	cmd := exec.Command(name, arg...)
	b, err := cmd.Output()
	// for debugging in Cloud Build
	if err != nil {
		if v, ok := err.(*exec.ExitError); ok {
			fmt.Println("ExitError: ", string(v.Stderr))
		}
	}
	Expect(err).To(BeNil())
	cmd.Process.Kill()
	cmd.Process.Release()
	return b
}

func CheckGlooHealthyWithDeployments(testHelper *helper.SoloTestHelper, deploymentNames []string) {
	for _, deploymentName := range deploymentNames {
		RunAndCleanCommand("kubectl", "rollout", "status", "deployment", "-n", testHelper.InstallNamespace, deploymentName)
	}
	kube2e.GlooctlCheckEventuallyHealthy(2, testHelper, "90s")
}

func CheckGlooOssHealthy(testHelper *helper.SoloTestHelper) {
	deploymentNames := []string{"gloo", "discovery", "gateway-proxy"}
	CheckGlooHealthyWithDeployments(testHelper, deploymentNames)
}

func GetGlooServerVersion(ctx context.Context, namespace string) (v string) {
	glooVersion, err := version.GetClientServerVersions(ctx, version.NewKube(namespace, ""))
	Expect(err).To(BeNil())
	Expect(len(glooVersion.GetServer())).To(Equal(1))
	for _, container := range glooVersion.GetServer()[0].GetKubernetes().GetContainers() {
		if v == "" {
			v = container.OssTag
		} else {
			Expect(container.OssTag).To(Equal(v))
		}
	}
	return v
}
