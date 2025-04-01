package convert

import (
	"fmt"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/gatewayapi/convert/domain"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/pflag"
)

type ErrorType string

const (
	ERROR_TYPE_UPDATE_OBJECT ErrorType = "UPDATE_OBJECT"
	ERROR_TYPE_NOT_SUPPORTED           = "NOT_SUPPORTED"
	ERROR_TYPE_IGNORED                 = "IGNORED"
	ERROR_UNKNOWN_REFERENCE            = "UNKNOWN_REFERENCE"
	ERROR_PARSE_ERROR                  = "PARSE_ERROR"
)

type Options struct {
	*options.Options
	InputFile               string
	InputDir                string
	GlooSnapshotFile        string
	OutputDir               string
	Stats                   bool
	CombineRouteOptions     bool
	RetainFolderStructure   bool
	IncludeUnknownResources bool
	DeleteOutputDir         bool
}

func (opts *Options) validate() error {

	count := 0
	if opts.InputDir != "" {
		count++
	}
	if opts.InputFile != "" {
		count++
	}
	if opts.GlooSnapshotFile != "" {
		count++
	}
	if count > 1 {
		return fmt.Errorf("only one of 'input-file' or 'directory' or 'input-snapshot' can be specified")
	}

	if !opts.DeleteOutputDir && folderExists(opts.OutputDir) {
		return fmt.Errorf("output-dir already %s exists. It can be deleted with --delete-output-dir", opts.OutputDir)
	}
	return nil
}
func folderExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
func (o *Options) addToFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.InputFile, "input-file", "", "Convert single file to Gateway API")
	flags.StringVar(&o.InputDir, "input-dir", "", "InputDir to read yaml/yml files recursively")
	flags.StringVar(&o.GlooSnapshotFile, "input-snapshot", "", "Gloo input snapshot file location")
	flags.BoolVar(&o.Stats, "print-stats", false, "Print stats about the conversion")
	flags.BoolVar(&o.CombineRouteOptions, "combine-route-options", false, "Combine routeOptions that are exactly the same and share them among the HTTPRoutes")
	flags.StringVar(&o.OutputDir, "output-dir", "./_output", "Output directory to write Gateway API configurations, it must not exist before or can be deleted/recreated with --recreate-output-dir")
	flags.BoolVar(&o.RetainFolderStructure, "retain-input-folder-structure", false, "When writing the output write the Gateway API configurations in the same folder structure they were read from (input-dir only)")
	flags.BoolVar(&o.IncludeUnknownResources, "include-unknown", false, "Copy unknown resources to output files (if files contain resources that are not Gloo APIs)")
	flags.BoolVar(&o.DeleteOutputDir, "delete-output-dir", false, "Delete the output directory if it already exists")
}

type GatewayAPIOutput struct {
	gatewayAPICache *domain.GatewayAPICache
	edgeCache       *domain.GlooEdgeCache
	errors          map[ErrorType][]GlooError
}

type GlooError struct {
	err       error
	errorType ErrorType
	name      string
	namespace string
	crdType   string
}

func (o *GatewayAPIOutput) AddError(errType ErrorType, msg string, args ...interface{}) {
	if o.errors == nil {
		o.errors = make(map[ErrorType][]GlooError)
	}
	if o.errors[errType] == nil {
		o.errors[errType] = make([]GlooError, 0)
	}

	o.errors[errType] = append(o.errors[errType], GlooError{
		err:       fmt.Errorf(msg, args...),
		errorType: errType,
		name:      "none",
		namespace: "none",
		crdType:   "none",
	})
}
func (o *GatewayAPIOutput) AddErrorFromWrapper(errType ErrorType, wrapper domain.Wrapper, msg string, args ...interface{}) {
	if o.errors == nil {
		o.errors = make(map[ErrorType][]GlooError)
	}
	if o.errors[errType] == nil {
		o.errors[errType] = make([]GlooError, 0)
	}
	o.errors[errType] = append(o.errors[errType], GlooError{
		err:       fmt.Errorf(msg, args...),
		errorType: errType,
		name:      wrapper.GetName(),
		namespace: wrapper.GetNamespace(),
		crdType:   fmt.Sprintf("%s/%s", wrapper.GetObjectKind().GroupVersionKind().Group, wrapper.GetObjectKind().GroupVersionKind().Kind),
	})
}

func NewGatewayAPIOutput() *GatewayAPIOutput {
	return &GatewayAPIOutput{
		gatewayAPICache: &domain.GatewayAPICache{},
		edgeCache:       &domain.GlooEdgeCache{},
	}
}
