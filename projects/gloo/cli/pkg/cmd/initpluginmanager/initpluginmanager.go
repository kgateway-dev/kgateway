package initpluginmanager

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/ghodss/yaml"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func Command(ctx context.Context) *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "init-plugin-manager",
		Short: "Install the Gloo Edge Enterprise CLI plugin manager",
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

	if _, err := os.Stat(filepath.Join(home, "bin")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	binFiles, err := os.ReadDir(filepath.Join(home, "bin"))
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
	tempDir, err := os.MkdirTemp("", "")
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
	binURL, err := getBinaryURL(ctx)
	if err != nil {
		return nil, err
	}
	binData, err := get(ctx, binURL)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(binPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := io.Copy(f, binData); err != nil {
		return nil, err
	}
	if err := f.Chmod(0755); err != nil {
		return nil, err
	}

	return &pluginBinary{path: binPath, home: home}, nil
}

func (binary pluginBinary) run(args ...string) error {
	cmd := exec.Command(binary.path, args...)
	cmd.Env = append(os.Environ(), "GLOOCTL_HOME="+binary.home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(out))
	}
	return err
}

type manifest struct {
	Versions []struct {
		Tag       string `json:"tag"`
		Platforms []struct {
			OS   string `json:"os"`
			Arch string `json:"arch"`
			URI  string `json:"uri"`
		} `json:"platforms"`
	} `json:"versions"`
}

func getBinaryURL(ctx context.Context) (string, error) {
	const manifestURL = "https://raw.githubusercontent.com/solo-io/glooctl-plugin-index/main/plugins/plugin.yaml"
	mfstBody, err := get(ctx, manifestURL)
	if err != nil {
		return "", err
	}
	defer mfstBody.Close()
	mfstData, err := io.ReadAll(mfstBody)
	if err != nil {
		return "", err
	}
	var mfst manifest
	if err := yaml.Unmarshal(mfstData, &mfst); err != nil {
		return "", err
	}
	cliVersion, err := versionutils.ParseVersion(version.Version)
	if err != nil {
		return "", err
	}
	for _, release := range mfst.Versions {
		v, err := versionutils.ParseVersion(release.Tag)
		if err != nil {
			fmt.Printf("invalid semver: %s\n", release.Tag)
			continue
		}
		if cliVersion.Major == v.Major && cliVersion.Minor == v.Minor {
			for _, platform := range release.Platforms {
				if platform.OS == runtime.GOOS && platform.Arch == runtime.GOARCH {
					return platform.URI, nil
				}
			}

			return "", eris.New("no compatible plugin manager binary found")
		}
	}

	return "", eris.New("no compatible plugin manager version found")
}

func get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	} else if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		if b, err := io.ReadAll(res.Body); err == nil {
			fmt.Println(string(b))
		} else {
			fmt.Printf("unable to read response body: %s\n", err.Error())
		}

		return nil, eris.Errorf("unexpected HTTP response: %s", res.Status)
	}

	return res.Body, nil
}
