package cmd

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	ConfigFileName = "glooctl-config.yaml"
	ConfigDirName  = ".gloo"

	defaultYaml = `# glooctl configuration file
# see https://gloo.solo.io/installation/advanced_configuration/glooctl-config/ for more information

`
	dirPermissions  = 0755
	filePermissions = 0644

	// this is kind of weird- we can't set cobra's default arg to "$HOME/..." and have it just work, because
	// it doesn't expand $HOME. We also can't set the default value to the expanded value of $HOME, ie something like
	// os.UserHomeDir(), because that will change the content of our generated docs/ directory based on whatever system
	// built glooctl last. So we settle for this placeholder.
	homeDir = "<home_directory>"

	// note that the available keys in this config file should be kept up to date in our public docs
	disableUsageReporting = "disableUsageReporting"
)

var DefaultConfigPath = path.Join(homeDir, ConfigDirName, ConfigFileName)

func ReadConfigFile(opts *options.Options, cmd *cobra.Command) error {
	configFilePathArg := opts.Top.ConfigFilePath

	configFilePath := ""
	if configFilePathArg == DefaultConfigPath {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		configFilePath = path.Join(homeDir, ConfigDirName, ConfigFileName)
	} else {
		configFilePath = configFilePathArg
	}

	err := ensureExists(configFilePath)
	if err != nil {
		return err
	}

	viper.SetConfigFile(configFilePath)
	viper.SetConfigType("yaml")
	err = viper.ReadInConfig()
	if err != nil {
		return err
	}

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
