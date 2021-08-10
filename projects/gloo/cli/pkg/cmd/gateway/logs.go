package gateway

import (
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

func logsCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use: "logs",
		Short: "dump Envoy logs from one of the proxy instances" +
			"" +
			"Note: this will enable verbose logging on Envoy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := getEnvoyLogs(opts); err != nil {
				return err
			}
			return nil
		},
	}

	pflags := cmd.PersistentFlags()
	pflags.BoolVarP(&opts.Proxy.DebugLogs, "debug", "d", true, "enable debug logging on the proxy as part of this command")
	pflags.BoolVarP(&opts.Proxy.FollowLogs, "follow", "f", false, "enable debug logging on the proxy as part of this command")
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func getEnvoyLogs(opts *options.Options) error {
	if opts.Proxy.DebugLogs {

		adminPort := strconv.Itoa(int(defaults.EnvoyAdminPort))
		portFwd := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.GetNamespace(),
			"deployment/"+opts.Proxy.Name, adminPort)
		portFwd.Stdout = os.Stderr
		portFwd.Stderr = os.Stderr
		if err := portFwd.Start(); err != nil {
			return errors.Wrapf(err, "failed to start port-forward")
		}
		done := make(chan struct{})
		errs := make(chan error)
		go func() {
			defer func() {
				if portFwd.Process != nil {
					portFwd.Process.Kill()
				}
			}()
			for {
				select {
				case <-opts.Top.Ctx.Done():
					return
				default:
				}
				res, err := http.Post("http://localhost:"+adminPort+"/logging?level=debug", "", nil)
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
				done <- struct{}{}
				return
			}
		}()
	waitForDebugMode:
		for {
			select {
			case <-opts.Top.Ctx.Done():
				return errors.Errorf("cancelled")
			case err := <-errs:
				log.Printf("connecting to envoy failed with err %v", err.Error())
			case <-time.After(time.Second * 30):
				return errors.Errorf("timed out trying to connect to Envoy admin port")
			case <-done:
				break waitForDebugMode
			}
		}
	}

	logsCmd := exec.Command("kubectl", "logs", "-n", opts.Metadata.GetNamespace(),
		"deployment/"+opts.Proxy.Name, "-c", opts.Proxy.Name)
	if opts.Proxy.FollowLogs {
		logsCmd.Args = append(logsCmd.Args, "-f")
	}
	logsCmd.Stdout = os.Stderr
	logsCmd.Stderr = os.Stderr
	if err := logsCmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to start port-forward")
	}
	return nil
}
