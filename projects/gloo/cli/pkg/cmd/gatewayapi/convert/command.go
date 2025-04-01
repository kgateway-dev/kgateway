package convert

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

const (
	RandomSuffix = 4
	RandomSeed   = 1
)

func RootCmd() *cobra.Command {
	opts := &Options{}
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert Gloo Edge APIs to Gateway API",
		Long:  "Convert Gloo Edge APIs to Gateway API by either providing kubernetes yaml files or a Gloo Gateway input snapshot",
		Example: `# This command converts Gloo Edge APIs to Gloo Gateway API yaml and places them in the '--output-dir' directory arranged by namespace.
# To generate gateway api by a single kubernetes yaml file. The 'output-dir'' must not exist
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output

# To delete and recreate the output dir add 'delete-output-dir'
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output --delete-output-dir

# To generate gateway api by a single kubernetes yaml file but place all the output configurations in the same file.
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output --retain-input-folder-structure

# To load a bunch of *.yaml or *.yml files in nested directories. You can also retain the same file structure which is helpful in CI/CD pipelines.
  glooctl gateway-api convert --input-dir ./gloo-configs --output-dir ./_output --retain-input-folder-structure

# Configuration can also be pulled an translated directly from a running Gloo Gateway Instance (version 1.17+).
# Port forward to the running gloo instance to grab its snapshot
  kubectl -n gloo-system port-forward deploy/gloo 9091
  curl localhost:9091/snapshots/input > gg-input.json
  
  glooctl gateway-api convert --input-snapshot gg-input.json --output-dir ./_output

# For all commands you can print stats about the migration such as number of configs
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output --print-stats

# If the yaml files contain non Gloo API yaml they can be retained by adding '--include-unknown'
  glooctl gateway-api convert --input-file gloo-yamls.yaml --output-dir ./_output --include-unknown`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.validate(); err != nil {
				return err
			}

			return run(opts)
		},
	}
	opts.addToFlags(cmd.PersistentFlags())
	cmd.SilenceUsage = true
	return cmd
}

func run(opts *Options) error {

	foundFiles, err := findFiles(opts)
	if err != nil {
		return err
	}

	filesMetrics.Add(float64(len(foundFiles)))

	output := NewGatewayAPIOutput()
	var isSnapshotFile bool
	if opts.GlooSnapshotFile != "" {
		isSnapshotFile = true
	}

	if err := output.Load(foundFiles, isSnapshotFile); err != nil {
		return err
	}

	// preprocessing
	if err := output.PreProcess(true); err != nil {
		return err
	}

	// now we need to convert the easy stuff like route tables
	// preprocessing
	if err := output.Convert(); err != nil {
		return err
	}

	// now we need to convert the easy stuff like route tables
	// preprocessing
	if err := output.PostProcess(opts); err != nil {
		return err
	}

	// now we need to convert the easy stuff like route tables
	// preprocessing
	if err := output.Write(opts); err != nil {
		return err
	}

	if opts.Stats {
		printMetrics(output)
	}

	return nil
}

func findFiles(opts *Options) ([]string, error) {
	var files []string
	if opts.InputDir != "" {
		fs, err := findYamlFiles(opts.InputDir)
		if err != nil {
			return nil, err
		}
		files = fs
	} else if opts.InputFile != "" {
		files = append(files, opts.InputFile)
	} else if opts.GlooSnapshotFile != "" {
		files = append(files, opts.GlooSnapshotFile)
	}

	return files, nil
}

func findYamlFiles(directory string) ([]string, error) {
	var files []string
	libRegEx, e := regexp.Compile("^.+\\.(yaml|yml)$")
	if e != nil {
		return nil, e
	}

	e = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err == nil && libRegEx.MatchString(info.Name()) {
			if !strings.Contains(info.Name(), "kustomization") {
				files = append(files, path)
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return files, nil
}
