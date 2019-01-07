package gateway

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

func dumpCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump",
		Short: "dump Envoy config from one of the gateway proxy instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgDump, err := getEnvoyCfgDump(opts)
			if err != nil {
				return err
			}
			fmt.Printf("%v", cfgDump)
			return nil
		},
	}
	flagutils.AddNamespaceFlag(cmd.PersistentFlags(), &opts.Metadata.Namespace)
	return cmd
}

func getEnvoyCfgDump(opts *options.Options) (string, error) {
	adminPort := strconv.Itoa(int(defaults.EnvoyAdminPort))
	portFwd := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.Namespace,
		"deployment/"+opts.Gateway.Proxy, adminPort)
	portFwd.Stdout = os.Stderr
	portFwd.Stderr = os.Stderr
	if err := portFwd.Start(); err != nil {
		return "", errors.Wrapf(err, "failed to start port-forward")
	}
	defer func() {
		if portFwd.Process != nil {
			portFwd.Process.Kill()
		}
	}()
	result := make(chan string)
	errs := make(chan error)
	go func() {
		for {
			select {
			case <-opts.Top.Ctx.Done():
				return
			default:
			}
			res, err := http.Get("http://localhost:" + adminPort + "/config_dump")
			if err != nil {
				errs <- err
				time.Sleep(time.Millisecond * 250)
				continue
			}
			if res.StatusCode != 200 {
				errs <- errors.Errorf("invalid status code: %v %v", res.StatusCode, res.Status)
				time.Sleep(time.Millisecond * 250)
				continue
			}
			b, err := ioutil.ReadAll(res.Body)
			if err != nil {
				errs <- err
				time.Sleep(time.Millisecond * 250)
				continue
			}
			res.Body.Close()
			result <- string(b)
			return
		}
	}()

	for {
		select {
		case <-opts.Top.Ctx.Done():
			return "", errors.Errorf("cancelled")
		case err := <-errs:
			log.Printf("connecting to envoy failed with err %v", err.Error())
		case res := <-result:
			return res, nil
		case <-time.After(time.Second * 3):
			return "", errors.Errorf("timed out trying to connect to Envoy admin port")
		}
	}

}

func statsCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "stats for one of the gateway proxy instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgDump, err := getEnvoyStatsDump(opts)
			if err != nil {
				return err
			}
			fmt.Printf("%v", cfgDump)
			return nil
		},
	}
	flagutils.AddNamespaceFlag(cmd.PersistentFlags(), &opts.Metadata.Namespace)
	return cmd
}

func getEnvoyStatsDump(opts *options.Options) (string, error) {
	adminPort := strconv.Itoa(int(defaults.EnvoyAdminPort))
	portFwd := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.Namespace,
		"deployment/"+opts.Gateway.Proxy, adminPort)
	portFwd.Stdout = os.Stderr
	portFwd.Stderr = os.Stderr
	if err := portFwd.Start(); err != nil {
		return "", errors.Wrapf(err, "failed to start port-forward")
	}
	defer func() {
		if portFwd.Process != nil {
			portFwd.Process.Kill()
		}
	}()
	result := make(chan string)
	errs := make(chan error)
	go func() {
		for {
			select {
			case <-opts.Top.Ctx.Done():
				return
			default:
			}
			res, err := http.Get("http://localhost:" + adminPort + "/stats")
			if err != nil {
				errs <- err
				time.Sleep(time.Millisecond * 250)
				continue
			}
			if res.StatusCode != 200 {
				errs <- errors.Errorf("invalid status code: %v %v", res.StatusCode, res.Status)
				time.Sleep(time.Millisecond * 250)
				continue
			}
			b, err := ioutil.ReadAll(res.Body)
			if err != nil {
				errs <- err
				time.Sleep(time.Millisecond * 250)
				continue
			}
			res.Body.Close()
			result <- string(b)
			return
		}
	}()

	for {
		select {
		case <-opts.Top.Ctx.Done():
			return "", errors.Errorf("cancelled")
		case err := <-errs:
			log.Printf("connecting to envoy failed with err %v", err.Error())
		case res := <-result:
			return res, nil
		case <-time.After(time.Second * 3):
			return "", errors.Errorf("timed out trying to connect to Envoy admin port")
		}
	}

}
