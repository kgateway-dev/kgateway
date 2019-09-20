package debug

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/solo-io/go-utils/tarutils"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/debugutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Filename = "/tmp/gloo-system-logs.tgz"
)

func DebugGlooSystemLogs(opts *options.Options, w io.Writer) error {
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
			logs := parseLogsFrom(response.Response, opts.Top.ErrorsOnly)
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

func parseLogsFrom(r io.ReadCloser, errorsOnly bool) strings.Builder {
	scanner := bufio.NewScanner(r)
	logs := strings.Builder{}
	for scanner.Scan() {
		line := scanner.Text()
		in := []byte(line)
		var raw map[string]interface{}
		if err := json.Unmarshal(in, &raw); err != nil {
			// is an envoy log, ignore for now.
			return strings.Builder{}
		}
		if errorsOnly {
			if raw["level"] == "error" {
				logs.WriteString(line + "\n")
			}
		} else {
			logs.WriteString(line + "\n")
		}
	}
	return logs
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

func DebugGlooSystemLogErrors() {
	// print out all the error logs from the gloo system
	fmt.Println("*** Start Gloo pods debug logs ***")
	opts := &options.Options{}
	opts.Top.ErrorsOnly = true
	err := DebugGlooSystemLogs(opts, os.Stdout)
	if err != nil {
		fmt.Println("getting Gloo log errors failed: %v", err)
	}
	fmt.Println("*** End Gloo pods debug logs ***")
}
