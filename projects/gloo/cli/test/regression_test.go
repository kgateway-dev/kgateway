package cli_unit_test

import (
	"bytes"
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/spf13/cobra"
)

var (
	commands = []string{
		constants.ADD_COMMAND.Use,
		constants.GET_COMMAND.Use,
		constants.DELETE_COMMAND.Use,
		constants.CREATE_COMMAND.Use,
	}

	uniqueCommands = []string{
		constants.PROXY_COMMAND.Use,
		constants.INSTALL_COMMAND.Use,
		constants.UPGRADE_COMMAND.Use,
	}
)

var _ = Describe("Regression", func() {

	var (
		app    *cobra.Command
		output *bytes.Buffer
		opts   *options.Options
	)

	BeforeEach(func() {
		opts = &options.Options{}
		preRunFuncs := []cmd.RunnableCommand{
			prerun.HarmonizeDryRunAndOutputFormat,
		}
		var postRunFuncs []cmd.RunnableCommand
		app = cmd.App(opts, preRunFuncs, postRunFuncs)
		output = &bytes.Buffer{}
		app.SetOutput(output)
	})

	It("can run properly without crashing", func() {
		// Otherwise uses args from ginkgo which crashes the test
		app.SetArgs([]string{})
		err := app.Execute()
		Expect(err).NotTo(HaveOccurred())
	})

	Context("All unique subcommands are callable", func() {
		for _, v := range uniqueCommands {
			It(fmt.Sprintf("can call %s subcommand", v), func() {
				app.SetArgs([]string{v, "help"})
				err := app.Execute()
				Expect(err).NotTo(HaveOccurred())
			})
		}
	})

	Context("All generic subcommands are callable", func() {
		for _, v := range commands {
			It(fmt.Sprintf("can call %s subcommand", v), func() {
				app.SetArgs([]string{v, "help"})
				err := app.Execute()
				Expect(err).NotTo(HaveOccurred())
			})
		}
	})

})
