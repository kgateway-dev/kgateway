package cmd

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	ConfigFileName = "glooctl-config.yaml"
	ConfigDirName  = ".gloo"

	defaultYaml = `# glooctl configuration file
# see https://gloo.solo.io/advanced_configuration/glooctl-config/ for more information

`
	dirPermissions  = 0755
	filePermissions = 0644

	// note that the available keys in this config file should be kept up to date in our public docs
	disableUsageReporting = "disableUsageReporting"
)

func ReadConfigFile(opts *options.Options, cmd *cobra.Command) error {
	configFilePath := opts.Top.ConfigFilePath

	err := ensureExists(opts.Top.ConfigFilePath)
	if err != nil {
		return err
	}

	viper.SetConfigFile(configFilePath)
	viper.SetConfigType("yaml")
	err = viper.ReadInConfig()
	if err != nil {
		return err
	}

	opts.Top.DisableUsageStatistics = viper.GetBool(disableUsageReporting)

	return err
}

// ensure that both the directory containing the file and the file itself exist
func ensureExists(fullConfigFilePath string) error {
	dir, _ := filepath.Split(fullConfigFilePath)

	err := os.MkdirAll(dir, dirPermissions)
	if err != nil {
		return err
	}

	_, err = os.Stat(fullConfigFilePath)
	if err != nil {
		// file does not exist
		return writeDefaultConfig(fullConfigFilePath)
	}

	// file exists
	return nil
}

func writeDefaultConfig(configPath string) error {
	return ioutil.WriteFile(configPath, []byte(defaultYaml), filePermissions)
}
