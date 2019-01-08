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
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

func logsCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use: "logs",
		Short: "dump Envoy logs from one of the gateway proxy instances" +
			"" +
			"Note: this will enable verbose logging on Envoy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := getEnvoyLogs(opts); err != nil {
				return err
			}
			return nil
		},
	}

	flagutils.AddNamespaceFlag(cmd.PersistentFlags(), &opts.Metadata.Namespace)
	pflags := cmd.PersistentFlags()
	pflags.BoolVarP(&opts.Gateway.DebugLogs, "debug", "d", true, "enable debug logging on the gateway proxy as part of this command")
	pflags.BoolVarP(&opts.Gateway.FollowLogs, "follow", "f", false, "enable debug logging on the gateway proxy as part of this command")
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func getEnvoyLogs(opts *options.Options) error {
	if opts.Gateway.DebugLogs {

		adminPort := strconv.Itoa(int(defaults.EnvoyAdminPort))
		portFwd := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.Namespace,
			"deployment/"+opts.Gateway.Proxy, adminPort)
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
			case <-time.After(time.Second * 3):
				return errors.Errorf("timed out trying to connect to Envoy admin port")
			case <-done:
				break waitForDebugMode
			}
		}
	}

	logsCmd := exec.Command("kubectl", "logs", "-n", opts.Metadata.Namespace,
		"deployment/"+opts.Gateway.Proxy, "-c", opts.Gateway.Proxy)
	if opts.Gateway.FollowLogs {
		logsCmd.Args = append(logsCmd.Args, "-f")
	}
	logsCmd.Stdout = os.Stderr
	logsCmd.Stderr = os.Stderr
	if err := logsCmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to start port-forward")
	}
	return nil
}
