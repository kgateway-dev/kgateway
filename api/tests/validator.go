package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"istio.io/istio/pkg/config/crd"
	"istio.io/istio/pkg/test/util/assert"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

func NewKgatewayValidator(t *testing.T) *crd.Validator {
	root := fsutils.GetModuleRoot()
	dirs := []string{filepath.Join(root, "pkg/kgateway/crds/gateway-crds.yaml")}
	dir, err := os.ReadDir(filepath.Join(root, "install/helm/kgateway-crds/templates/"))
	assert.NoError(t, err)
	agentgatewayDir, err := os.ReadDir(filepath.Join(root, "install/helm/agentgateway-crds/templates/"))
	assert.NoError(t, err)
	for _, d := range dir {
		if strings.HasSuffix(d.Name(), ".yaml") {
			dirs = append(dirs, filepath.Join(root, "install/helm/kgateway-crds/templates", d.Name()))
		}
	}
	for _, d := range agentgatewayDir {
		if strings.HasSuffix(d.Name(), ".yaml") {
			dirs = append(dirs, filepath.Join(root, "install/helm/agentgateway-crds/templates", d.Name()))
		}
	}
	v, err := crd.NewValidatorFromFiles(
		dirs...,
	)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
