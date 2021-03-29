package initpluginmanager

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/rotisserie/eris"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const tempBinaryVersion = "v1.7.0" // TODO(ryantking): Pin this to first released version of plugin manager

func Command(ctx context.Context) *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "init-plugin-manager",
		Short: "Install the plugin manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := opts.getHome()
			if err != nil {
				return err
			}
			if err := checkExisting(home, opts.force); err != nil {
				return err
			}
			binary, err := downloadTempBinary(ctx, home)
			if err != nil {
				return err
			}
			const defaultIndexURL = "https://github.com/solo-io/glooctl-plugin-index.git"
			if err := binary.run("index", "add", "default", defaultIndexURL); err != nil {
				return err
			}
			if err := binary.run("install", "plugin"); err != nil {
				return err
			}
			homeStr := opts.home
			if homeStr == "" {
				homeStr = "$HOME/.gloo"
			}
			fmt.Printf(`The glooctl plugin manager was successfully installed 🎉
Add the glooctl plugins to your path with:
  export PATH=%s/bin:$PATH
Now run:
  glooctl plugin --help     # see the commands available to you
Please see visit the Gloo Edge website for more info:  https://www.solo.io/products/gloo-edge/
`, homeStr)
			return nil
		},
	}
	opts.addToFlags(cmd.Flags())
	cmd.SilenceUsage = true
	return cmd
}

type options struct {
	home  string
	force bool
}

func (o *options) addToFlags(flags *pflag.FlagSet) {
	flags.StringVar(&o.home, "gloo-home", "", "Gloo home directory (default: $HOME/.gloo)")
	flags.BoolVarP(&o.force, "force", "f", false, "Delete any existing plugin data if found and reinitialize")
}

func (o options) getHome() (string, error) {
	if o.home != "" {
		return o.home, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userHome, ".gloo"), nil
}

func checkExisting(home string, force bool) error {
	pluginDirs := []string{"index", "receipts", "store"}
	dirty := false
	for _, dir := range pluginDirs {
		if _, err := os.Stat(filepath.Join(home, dir)); err == nil {
			dirty = true
			break
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if !dirty {
		return nil
	}
	if !force {
		return eris.Errorf("found existing plugin manager files in %s, rerun with -f to delete and reinstall", home)
	}
	for _, dir := range pluginDirs {
		os.RemoveAll(filepath.Join(home, dir))
	}
	binFiles, err := ioutil.ReadDir(filepath.Join(home, "bin"))
	if err != nil {
		return err
	}
	for _, file := range binFiles {
		if file.Name() != "glooctl" {
			os.Remove(filepath.Join(home, "bin", file.Name()))
		}
	}
	return nil
}

type pluginBinary struct {
	path string
	home string
}

func downloadTempBinary(ctx context.Context, home string) (*pluginBinary, error) {
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		return nil, err
	}
	binPath := filepath.Join(tempDir, "plugin")
	if runtime.GOARCH != "amd64" {
		return nil, eris.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, eris.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	url := fmt.Sprintf(
		"https://storage.googleapis.com/gloo-ee/glooctl-plugins/plugin/%s/glooctl-plugin-%s-%s",
		tempBinaryVersion, runtime.GOOS, runtime.GOARCH,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		fmt.Println(string(b))
		return nil, eris.Errorf("could not download plugin manager binary: %d %s", res.StatusCode, res.Status)
	}
	if err := ioutil.WriteFile(binPath, b, 0755); err != nil {
		return nil, err
	}
	return &pluginBinary{path: binPath, home: home}, nil
}

func (binary pluginBinary) run(args ...string) error {
	cmd := exec.Command(binary.path, args...)
	cmd.Env = append(cmd.Env, "GLOOCTL_HOME="+binary.home)
	return cmd.Run()
}
