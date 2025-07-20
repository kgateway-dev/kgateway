package e2e

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/helmutils"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/actions"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/assertions"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/cluster"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/helper"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/install"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/runtime"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// CreateTestInstallation is the simplest way to construct a TestInstallation in kgateway.
// It is syntactic sugar on top of CreateTestInstallationForCluster
func CreateTestInstallation(
	t *testing.T,
	installContext *install.Context,
) *TestInstallation {
	runtimeContext := testruntime.NewContext()
	clusterContext := cluster.MustKindContext(runtimeContext.ClusterName)

	if err := install.ValidateInstallContext(installContext); err != nil {
		// We error loudly if the context is misconfigured
		panic(err)
	}

	return CreateTestInstallationForCluster(t, runtimeContext, clusterContext, installContext)
}

// CreateTestInstallationForCluster is the standard way to construct a TestInstallation
// It accepts context objects from 3 relevant sources:
//
//	runtime - These are properties that are supplied at runtime and will impact how tests are executed
//	cluster - These are properties that are used to connect to the Kubernetes cluster
//	install - These are properties that are relevant to how the kgateway installation will be configured
func CreateTestInstallationForCluster(
	t *testing.T,
	runtimeContext testruntime.Context,
	clusterContext *cluster.Context,
	installContext *install.Context,
) *TestInstallation {
	installation := &TestInstallation{
		// RuntimeContext contains the set of properties that are defined at runtime by whoever is invoking tests
		RuntimeContext: runtimeContext,

		// ClusterContext contains the metadata about the Kubernetes Cluster that is used for this TestCluster
		ClusterContext: clusterContext,

		// Maintain a reference to the Metadata used for this installation
		Metadata: installContext,

		// Create an actions provider, and point it to the running installation
		Actions: actions.NewActionsProvider().
			WithClusterContext(clusterContext).
			WithInstallContext(installContext),

		// Create an assertions provider, and point it to the running installation
		Assertions: assertions.NewProvider(t).
			WithClusterContext(clusterContext).
			WithInstallContext(installContext),

		// GeneratedFiles contains the unique location where files generated during the execution
		// of tests against this installation will be stored
		// By creating a unique location, per TestInstallation and per Cluster.Name we guarantee isolation
		// between TestInstallation outputs per CI run
		GeneratedFiles: MustGeneratedFiles(installContext.InstallNamespace, clusterContext.Name),
	}
	runtime.SetFinalizer(installation, func(i *TestInstallation) { i.finalize() })
	return installation
}

// TestInstallation is the structure around a set of tests that validate behavior for an installation
// of kgateway.
type TestInstallation struct {
	fmt.Stringer

	// RuntimeContext contains the set of properties that are defined at runtime by whoever is invoking tests
	RuntimeContext testruntime.Context

	// ClusterContext contains the metadata about the Kubernetes Cluster that is used for this TestCluster
	ClusterContext *cluster.Context

	// Metadata contains the properties used to install kgateway
	Metadata *install.Context

	// Actions is the entity that creates actions that can be executed by the Operator
	Actions *actions.Provider

	// Assertions is the entity that creates assertions that can be executed by the Operator
	Assertions *assertions.Provider

	// GeneratedFiles is the collection of directories and files that this test installation _may_ create
	GeneratedFiles GeneratedFiles

	// IstioctlBinary is the path to the istioctl binary that can be used to interact with Istio
	IstioctlBinary string
}

func (i *TestInstallation) String() string {
	return i.Metadata.InstallNamespace
}

func (i *TestInstallation) finalize() {
	if err := os.RemoveAll(i.GeneratedFiles.TempDir); err != nil {
		panic(fmt.Sprintf("Failed to remove temporary directory: %s", i.GeneratedFiles.TempDir))
	}
}

func (i *TestInstallation) AddIstioctl(ctx context.Context) error {
	istioctl, err := cluster.GetIstioctl(ctx)
	if err != nil {
		return fmt.Errorf("failed to download istio: %w", err)
	}
	i.IstioctlBinary = istioctl
	return nil
}

func (i *TestInstallation) InstallMinimalIstio(ctx context.Context) error {
	return cluster.InstallMinimalIstio(ctx, i.IstioctlBinary, i.ClusterContext.KubeContext)
}

func (i *TestInstallation) InstallRevisionedIstio(ctx context.Context, rev, profile string, extraArgs ...string) error {
	return cluster.InstallRevisionedIstio(ctx, i.IstioctlBinary, i.ClusterContext.KubeContext, rev, profile, extraArgs...)
}

func (i *TestInstallation) UninstallIstio() error {
	return cluster.UninstallIstio(i.IstioctlBinary, i.ClusterContext.KubeContext)
}

func (i *TestInstallation) CreateIstioBugReport(ctx context.Context) {
	cluster.CreateIstioBugReport(ctx, i.IstioctlBinary, i.ClusterContext.KubeContext, i.GeneratedFiles.FailureDir)
}

func (i *TestInstallation) InstallKgatewayFromLocalChart(ctx context.Context) {
	i.InstallKgatewayCRDsFromLocalChart(ctx)
	i.InstallKgatewayCoreFromLocalChart(ctx)
}

func (i *TestInstallation) InstallKgatewayCRDsFromLocalChart(ctx context.Context) {
	if testutils.ShouldSkipInstall() {
		return
	}

	// install the CRD chart first
	crdChartURI, err := helper.GetLocalChartPath(helmutils.CRDChartName)
	i.Assertions.Require.NoError(err)
	err = i.Actions.Helm().WithReceiver(os.Stdout).Upgrade(
		ctx,
		helmutils.InstallOpts{
			CreateNamespace: true,
			ReleaseName:     helmutils.CRDChartName,
			Namespace:       i.Metadata.InstallNamespace,
			ChartUri:        crdChartURI,
		})
	i.Assertions.Require.NoError(err)
}

func (i *TestInstallation) InstallKgatewayCoreFromLocalChart(ctx context.Context) {
	if testutils.ShouldSkipInstall() {
		return
	}

	// and then install the main chart
	chartUri, err := helper.GetLocalChartPath(helmutils.ChartName)
	i.Assertions.Require.NoError(err)
	err = i.Actions.Helm().WithReceiver(os.Stdout).Upgrade(
		ctx,
		helmutils.InstallOpts{
			Namespace:       i.Metadata.InstallNamespace,
			CreateNamespace: true,
			ValuesFiles:     []string{i.Metadata.ProfileValuesManifestFile, i.Metadata.ValuesManifestFile},
			ReleaseName:     helmutils.ChartName,
			ChartUri:        chartUri,
			ExtraArgs:       i.Metadata.ExtraHelmArgs,
		})
	i.Assertions.Require.NoError(err)
	i.Assertions.EventuallyKgatewayInstallSucceeded(ctx)
}

// TODO implement this when we add upgrade tests
// func (i *TestInstallation) InstallKgatewayFromRelease(ctx context.Context, version string) {
// 	if testutils.ShouldSkipInstall() {
// 		return
// 	}
// }

func (i *TestInstallation) UninstallKgateway(ctx context.Context) {
	i.UninstallKgatewayCore(ctx)
	i.UninstallKgatewayCRDs(ctx)
}

func (i *TestInstallation) UninstallKgatewayCore(ctx context.Context) {
	if testutils.ShouldSkipInstall() {
		return
	}

	// uninstall the main chart first
	err := i.Actions.Helm().Uninstall(
		ctx,
		helmutils.UninstallOpts{
			Namespace:   i.Metadata.InstallNamespace,
			ReleaseName: helmutils.ChartName,
		},
	)
	i.Assertions.Require.NoError(err, "failed to uninstall main chart")
	i.Assertions.EventuallyKgatewayUninstallSucceeded(ctx)
}

func (i *TestInstallation) UninstallKgatewayCRDs(ctx context.Context) {
	if testutils.ShouldSkipInstall() {
		return
	}

	// uninstall the CRD chart
	err := i.Actions.Helm().Uninstall(
		ctx,
		helmutils.UninstallOpts{
			Namespace:   i.Metadata.InstallNamespace,
			ReleaseName: helmutils.CRDChartName,
		},
	)
	i.Assertions.Require.NoError(err, "failed to uninstall CRD chart")
}

// PreFailHandler is the function that is invoked if a test in the given TestInstallation fails
func (i *TestInstallation) PreFailHandler(ctx context.Context) {
	// The idea here is we want to accumulate ALL information about this TestInstallation into a single directory
	// That way we can upload it in CI, or inspect it locally

	failureDir := i.GeneratedFiles.FailureDir
	err := os.Mkdir(failureDir, os.ModePerm)
	// We don't want to fail on the output directory already existing. This could occur
	// if multiple tests running in the same cluster from the same installation namespace
	// fail.
	if err != nil && !errors.Is(err, fs.ErrExist) {
		i.Assertions.Require.NoError(err)
	}

	// The kubernetes/e2e tests may use multiple namespaces, so we need to dump all of them
	namespaces, err := i.Actions.Kubectl().Namespaces(ctx)
	i.Assertions.Require.NoError(err)

	// Dump the logs and state of the cluster
	helpers.StandardKgatewayDumpOnFail(os.Stdout, failureDir, namespaces)()
}

// GeneratedFiles is a collection of files that are generated during the execution of a set of tests
type GeneratedFiles struct {
	// TempDir is the directory where any temporary files should be created
	// Tests may create files for any number of reasons:
	// - A: When a test renders objects in a file, and then uses this file to create and delete values
	// - B: When a test invokes a command that produces a file as a side effect (glooctl, for example)
	// Files in this directory are an implementation detail of the test itself.
	// As a result, it is the callers responsibility to clean up the TempDir when the tests complete
	TempDir string

	// FailureDir is the directory where any assets that are produced on failure will be created
	FailureDir string
}

// MustGeneratedFiles returns GeneratedFiles, or panics if there was an error generating the directories
func MustGeneratedFiles(tmpDirId, clusterId string) GeneratedFiles {
	tmpDir, err := os.MkdirTemp("", tmpDirId)
	if err != nil {
		panic(err)
	}

	// output path is in the format of bug_report/cluster_name/tmp_dir_id
	failureDir := filepath.Join(testruntime.PathToBugReport(), clusterId, tmpDirId)
	err = os.MkdirAll(failureDir, os.ModePerm)
	if err != nil {
		panic(err)
	}

	return GeneratedFiles{
		TempDir:    tmpDir,
		FailureDir: failureDir,
	}
}
