package debug

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/solo-io/gloo/pkg/cliutil/install"
	installcmd "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/install"

	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/solo-io/go-utils/tarutils"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/debugutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Filename        = "/tmp/gloo-system-logs.tgz"
	filePermissions = 0644
)

func DebugLogs(opts *options.Options, w io.Writer) error {
	responses, err := setup(opts)
	if err != nil {
		return err
	}

	fs := afero.NewOsFs()
	dir, err := afero.TempDir(fs, "", "")
	if err != nil {
		return err
	}
	defer fs.RemoveAll(dir)
	storageClient := debugutils.NewFileStorageClient(fs)

	eg := errgroup.Group{}
	for _, response := range responses {
		response := response
		eg.Go(func() error {
			defer response.Response.Close()
			var logs strings.Builder
			if opts.Top.ErrorsOnly {
				logs = utils.FilterLogLevel(response.Response, utils.LogLevelError)
			} else {
				logs = utils.FilterLogLevel(response.Response, utils.LogLevelAll)
			}
			if logs.Len() > 0 {
				if opts.Top.Zip {
					err = storageClient.Save(dir, &debugutils.StorageObject{
						Resource: strings.NewReader(logs.String()),
						Name:     response.ResourceId(),
					})
				} else {
					err = displayLogs(w, logs)
					if err != nil {
						return err
					}
				}
			}
			return nil
		})
	}
	err = eg.Wait()
	if err != nil {
		return err
	}

	if opts.Top.Zip {
		if opts.Top.File == "" {
			opts.Top.File = Filename
		}
		err = zip(fs, dir, opts.Top.File)
		if err != nil {
			return err
		}
	}

	return nil
}

func DebugYaml(opts *options.Options, w io.Writer) error {
	return DumpYaml(opts.Top.File, opts.Metadata.Namespace, &install.CmdKubectl{})
}

// visible for testing
func DumpYaml(fileToWrite, namespace string, kubeCli install.KubeCli) error {

	var manifests []string
	for _, kind := range installcmd.GlooNamespacedKinds {
		output, err := kubeCli.KubectlOut(nil, "get", kind, "-oyaml", "-n", namespace)
		if err != nil {
			return err
		}
		manifests = append(manifests, string(output))
	}

	for _, crd := range installcmd.GlooCrdNames {
		output, err := kubeCli.KubectlOut(nil, "get", crd, "-oyaml", "-n", namespace)
		if err != nil {
			return err
		}
		manifests = append(manifests, string(output))
	}

	allOutput := strings.Join(manifests, "\n---\n")
	if fileToWrite == "" {
		_, err := fmt.Fprint(os.Stdout, allOutput)
		return err
	} else {
		return ioutil.WriteFile(fileToWrite, []byte(allOutput), filePermissions)
	}
}

func zip(fs afero.Fs, dir string, file string) error {
	tarball, err := fs.Create(file)
	if err != nil {
		return err
	}
	if err := tarutils.Tar(dir, fs, tarball); err != nil {
		return err
	}
	_, err = tarball.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	return nil
}

func displayLogs(w io.Writer, logs strings.Builder) error {
	_, err := fmt.Fprintf(w, logs.String())
	return err
}

func setup(opts *options.Options) ([]*debugutils.LogsResponse, error) {
	pods, err := helpers.MustKubeClient().CoreV1().Pods(opts.Metadata.Namespace).List(metav1.ListOptions{
		LabelSelector: "gloo",
	})
	if err != nil {
		return nil, err
	}
	resources, err := debugutils.ConvertPodsToUnstructured(pods)
	if err != nil {
		return nil, err
	}
	logCollector, err := debugutils.DefaultLogCollector()
	if err != nil {
		return nil, err
	}
	logRequests, err := logCollector.GetLogRequests(resources)
	if err != nil {
		return nil, err
	}

	return logCollector.LogRequestBuilder.StreamLogs(logRequests)
}
