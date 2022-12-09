package cmd

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	ConfigFileName = "glooctl-config.yaml"
	ConfigDirName  = ".gloo"

	defaultYaml = `# glooctl configuration file
# see https://gloo.solo.io/installation/advanced_configuration/glooctl-config/ for more information

# The maximum length of time to wait before giving up on a secret request. A value of zero means no timeout.
secretClientTimeoutSeconds: 30

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

	checkTimeoutSeconds                  = "checkTimeoutSeconds"
	checkConnectionTimeoutSeconds        = "checkConnectionTimeoutSeconds"
	defaultTimeoutSeconds                = "defaultTimeoutSeconds"
	deploymentClientSeconds              = "deploymentClientSeconds"
	podClientTimeoutSeconds              = "podClientTimeoutSeconds"
	settingsClientTimeoutSeconds         = "settingsClientTimeoutSeconds "
	upstreamsClientTimeoutSeconds        = "upstreamsClientTimeoutSeconds"
	upstreamGroupsClientTimeoutSeconds   = "upstreamGroupsClientTimeoutSeconds"
	authConfigsClientTimeoutSeconds      = "authConfigsClientTimeoutSeconds"
	rateLimitConfigsClientTimeoutSeconds = "rateLimitConfigsClientTimeoutSeconds"
	virtualHostOptionsClientSeconds      = "virtualHostOptionsClientSeconds"
	routeOptionsClientSeconds            = "routeOptionsClientSeconds"
	secretClientTimeoutSeconds           = "secretClientTimeoutSeconds"
	virtualServicesClientTimeoutSeconds  = "virtualServicesClientTimeoutSeconds"
	gatewaysClientTimeoutSeconds         = "gatewaysClientTimeoutSeconds"
	proxyClientTimeoutSeconds            = "proxyClientTimeoutSeconds"
	xdsMetricsTimeoutSeconds             = "xdsMetricsTimeoutSeconds"
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

	loadValuesIntoOptions(opts)
	return nil
}

func stringToDuration(str string) time.Duration {
	return time.Duration(viper.GetInt64(str)) * time.Second
}

func stringToDurationWithDefault(str, defaultString string) time.Duration {
	if viper.GetInt64(str) == 0 {
		return stringToDuration(defaultString)
	}
	return stringToDuration(str)
}

// Assigns values from config file (or default) into the provided Options struct
func loadValuesIntoOptions(opts *options.Options) {
	viper.SetDefault(defaultTimeoutSeconds, 0)

	opts.Check = options.Check{
		CheckTimeout:                    stringToDuration("0s"),
		CheckConnectionTimeout:          stringToDurationWithDefault(checkConnectionTimeoutSeconds, defaultTimeoutSeconds),
		DefaultTimeout:                  stringToDuration(defaultTimeoutSeconds),
		DeploymentClientTimeout:         stringToDurationWithDefault(deploymentClientSeconds, defaultTimeoutSeconds),
		PodClientTimeout:                stringToDurationWithDefault(podClientTimeoutSeconds, defaultTimeoutSeconds),
		SettingsClientTimeout:           stringToDurationWithDefault(settingsClientTimeoutSeconds, defaultTimeoutSeconds),
		UpstreamsClientTimeout:          stringToDurationWithDefault(upstreamsClientTimeoutSeconds, defaultTimeoutSeconds),
		UpstreamGroupsClientTimeout:     stringToDurationWithDefault(upstreamGroupsClientTimeoutSeconds, defaultTimeoutSeconds),
		AuthConfigsClientTimeout:        stringToDurationWithDefault(authConfigsClientTimeoutSeconds, defaultTimeoutSeconds),
		RateLimitConfigsClientTimeout:   stringToDurationWithDefault(rateLimitConfigsClientTimeoutSeconds, defaultTimeoutSeconds),
		VirtualHostOptionsClientTimeout: stringToDurationWithDefault(virtualHostOptionsClientSeconds, defaultTimeoutSeconds),
		RouteOptionsClientTimeout:       stringToDurationWithDefault(routeOptionsClientSeconds, defaultTimeoutSeconds),
		SecretClientTimeout:             stringToDurationWithDefault(secretClientTimeoutSeconds, "30s"),
		VirtualServicesClientTimeout:    stringToDurationWithDefault(virtualServicesClientTimeoutSeconds, defaultTimeoutSeconds),
		GatewaysClientTimeout:           stringToDurationWithDefault(gatewaysClientTimeoutSeconds, defaultTimeoutSeconds),
		ProxyClientTimeout:              stringToDurationWithDefault(proxyClientTimeoutSeconds, defaultTimeoutSeconds),
		XdsMetricsTimeout:               stringToDurationWithDefault(xdsMetricsTimeoutSeconds, defaultTimeoutSeconds),
	}
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
