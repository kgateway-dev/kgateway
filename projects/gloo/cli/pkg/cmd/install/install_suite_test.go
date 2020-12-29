package install_test

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
	"strings"

	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gotestutils "github.com/solo-io/go-utils/testutils"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func TestInstall(t *testing.T) {
	RegisterFailHandler(Fail)
	gotestutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Install Suite")
}

var RootDir string
var dir string
var file, values1, values2 string

const (
	unitTestingTaggedVersion = "v1.6.0-vunit-testing"
	expectedHelmFilename     = "gloo-unit-testing.tgz"
)

// NOTE: This needs to be run from the root of the repo as the working directory
var _ = BeforeSuite(func() {

	// Make sure we don't hit a real cluster during any of the tests in this suite
	helpers.UseMemoryClients()

	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	RootDir = filepath.Join(cwd, "../../../../../..")

	// the regression test depend on having only one chart in _test.
	// so run these in a different location.
	dir = filepath.Join(RootDir, "_unit_test/")
	os.Mkdir(dir, 0755)

	err = testutils.Make(RootDir, "build-test-chart TEST_ASSET_DIR=\""+dir+"\" TAGGED_VERSION="+unitTestingTaggedVersion)
	Expect(err).NotTo(HaveOccurred())

	// Some tests need the Gloo/GlooE version that gets linked into the glooctl binary at build time
	err = testutils.Make(RootDir, "glooctl")
	Expect(err).NotTo(HaveOccurred())

	file = filepath.Join(dir, expectedHelmFilename)

	values1 = filepath.Join(dir, "values-namespace1.yaml")
	values2 = filepath.Join(dir, "values-namespace2.yaml")
	f, err := os.Create(values1)
	Expect(err).NotTo(HaveOccurred())
	_, err = f.WriteString(`
settings:
 writeNamespace: test-namespace`)
	Expect(err).NotTo(HaveOccurred())
	f.Close()

	f2, err := os.Create(values2)
	Expect(err).NotTo(HaveOccurred())
	_, err = f2.WriteString(`
settings:
 writeNamespace: test-namespace-2`)
	Expect(err).NotTo(HaveOccurred())
	f2.Close()

	// Make sure all crds are included
	crdDir := filepath.Join(RootDir, "/install/helm/gloo/crds")
	files, err := ioutil.ReadDir(crdDir)
	Expect(err).NotTo(HaveOccurred())
	var crdNames []string
	for _, f := range files {
		ext := filepath.Ext(f.Name())
		if !f.IsDir() && (strings.EqualFold(ext, ".yaml") || strings.EqualFold(ext, ".yml") || strings.EqualFold(ext, ".json")) {
			manifest, err := ioutil.ReadFile(crdDir + "/" + f.Name())
			Expect(err).NotTo(HaveOccurred())
			jsn, err := yaml.YAMLToJSON(manifest)
			Expect(err).NotTo(HaveOccurred())
			runtimeObj, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jsn)
			Expect(err).NotTo(HaveOccurred())

			resource := runtimeObj.(*unstructured.Unstructured)
			crdNames = append(crdNames, resource.GetName())
		}
	}
	Expect(crdNames).To(ContainElements(install.GlooCrdNames))
})

var _ = AfterSuite(func() {
	err := os.Remove(file)
	Expect(err).NotTo(HaveOccurred())
	err = os.Remove(values1)
	Expect(err).NotTo(HaveOccurred())
	err = os.Remove(values2)
	Expect(err).NotTo(HaveOccurred())
	err = os.RemoveAll(dir)
	Expect(err).NotTo(HaveOccurred())
})
