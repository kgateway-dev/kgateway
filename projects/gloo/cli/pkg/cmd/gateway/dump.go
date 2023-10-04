package gateway

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/solo-io/go-utils/cliutils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

func dumpCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump",
		Short: "dump Envoy config from one of the proxy instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgDump, err := getEnvoyCfgDump(opts)
			if err != nil {
				return err
			}
			fmt.Printf("%v", cfgDump)
			return nil
		},
	}
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func getEnvoyCfgDump(opts *options.Options) (string, error) {
	adminPort := strconv.Itoa(int(defaults.EnvoyAdminPort))
	portFwd := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.GetNamespace(),
		"deployment/"+opts.Proxy.Name, adminPort)
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
			b, err := io.ReadAll(res.Body)
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

	timer := time.Tick(time.Second * 5)

	for {
		select {
		case <-opts.Top.Ctx.Done():
			return "", errors.Errorf("cancelled")
		case err := <-errs:
			log.Printf("connecting to envoy failed with err %v", err.Error())
		case res := <-result:
			return res, nil
		case <-timer:
			return "", errors.Errorf("timed out trying to connect to Envoy admin port")
		}
	}

}

func GetEnvoyCfgDump(opts *options.Options) (string, error) {
	return getEnvoyCfgDump(opts)
}

func statsCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "stats for one of the proxy instances",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgDump, err := getEnvoyStatsDump(opts)
			if err != nil {
				return err
			}
			fmt.Printf("%v", cfgDump)
			return nil
		},
	}
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func getEnvoyStatsDump(opts *options.Options) (string, error) {
	adminPort := strconv.Itoa(int(defaults.EnvoyAdminPort))
	portFwd := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.GetNamespace(),
		"deployment/"+opts.Proxy.Name, adminPort)
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
			b, err := io.ReadAll(res.Body)
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
		case <-time.After(time.Second * 30):
			return "", errors.Errorf("timed out trying to connect to Envoy admin port")
		}
	}

}
