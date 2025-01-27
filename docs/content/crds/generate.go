package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/kgateway-dev/kgateway/pkg/utils/cmdutils"
	"github.com/kgateway-dev/kgateway/projects/gateway2/api/v1alpha1"
	"github.com/rotisserie/eris"
	"github.com/stoewer/go-strcase"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func main() {
	err := generateCrdReferenceDocs(context.Background())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// generateCrdReferenceDocs is the entrypoint to our automation for updating CRD reference markdown files
func generateCrdReferenceDocs(ctx context.Context) error {
	gvks := []schema.GroupVersionKind{
		v1alpha1.GatewayParametersGVK,
		v1alpha1.DirectResponseGVK,
	}

	outErr := &multierror.Error{}
	for _, gvk := range gvks {
		err := generateCrdReferenceMarkdown(ctx, gvk)
		outErr = multierror.Append(outErr, err)
	}

	return outErr.ErrorOrNil()
}

func generateCrdReferenceMarkdown(ctx context.Context, gvk schema.GroupVersionKind) error {
	// sourceFile is the path to the CRD. This is used as the source of truth for the CRD reference docs
	sourceFile := filepath.Join(
		"install",
		"helm",
		"gloo",
		"crds",
		fmt.Sprintf("%s_%s.yaml", gvk.Group, strings.ToLower(kindPlural(gvk))),
	)

	// outputFile is the path to the generated reference markdown file.
	// NOTE: For now, this is tightly coupled to the `gateway2` project, since that is where the APIs that we
	// rely on are defined, though that may need to change in the future
	outputFile := filepath.Join(
		"docs",
		"content",
		"reference",
		"api",
		"github.com",
		"solo-io",
		"gloo",
		"projects",
		"gateway2",
		"api",
		gvk.Version,
		fmt.Sprintf("%s.md", strcase.SnakeCase(kindPlural(gvk))))

	// templateFile is the path to the file used as the template for our docs
	templateFile := filepath.Join(
		"docs",
		"content",
		"crds",
		"templates",
		"markdown.tmpl")

	cmd := cmdutils.Command(ctx, "go", "run", "fybrik.io/crdoc",
		"--resources",
		sourceFile,
		"--output",
		outputFile,
		"--template",
		templateFile,
	)
	runErr := cmd.Run()

	if runErr.Cause() != nil {
		return eris.Wrapf(runErr.Cause(), "%s produced error %s", runErr.PrettyCommand(), runErr.Error())
	}
	return nil
}

// kindPlural returns the pluralized kind for a given GVK.
// This is hacky, but is useful because CRD files are named using this format, so we need a way to look up that file name
// If the name of the file is incorrect, a developer will realize this because the script will fail with a file not found error.
func kindPlural(gvk schema.GroupVersionKind) string {
	// ensure that kind which ends in s, is not duplicated
	// ie GatewayParameters becomes GatewayParameters, not GatewayParameterss
	return strings.TrimSuffix(gvk.Kind, "s") + "s"
}
