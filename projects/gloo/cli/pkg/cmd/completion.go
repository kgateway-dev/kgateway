package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	completionLong = `
	Output shell completion code for the specified shell (bash or zsh).
	The shell code must be evaluated to provide interactive
	completion of glooctl commands.  This can be done by sourcing it from
	the .bash_profile.
	Note for zsh users: [1] zsh completions are only supported in versions of zsh >= 5.2`

	completionExample = `
	# Installing bash completion on macOS using homebrew
	## If running Bash 3.2 included with macOS
	  	brew install bash-completion
	## or, if running Bash 4.1+
	    brew install bash-completion@2
	## You may need add the completion to your completion directory
	    glooctl completion bash > $(brew --prefix)/etc/bash_completion.d/glooctl
	# Installing bash completion on Linux
	## Load the glooctl completion code for bash into the current shell
	    source <(glooctl completion bash)
	## Write bash completion code to a file and source if from .bash_profile
	    glooctl completion bash > ~/.glooctl/completion.bash.inc
	    printf "
 	     # glooctl shell completion
	      source '$HOME/.glooctl/completion.bash.inc'
	      " >> $HOME/.bash_profile
	    source $HOME/.bash_profile
	# Load the glooctl completion code for zsh[1] into the current shell
	    source <(glooctl completion zsh)
	# Set the glooctl completion code for zsh[1] to autoload on startup
	    glooctl completion zsh > "${fpath[1]}/_glooctl"`
)

func completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion SHELL",
		Short:     "generate auto completion for your shell",
		Long:      completionLong,
		Example:   completionExample,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh"},
		Run: func(c *cobra.Command, a []string) {
			switch strings.ToLower(a[0]) {
			case "bash":
				if err := c.Parent().GenBashCompletion(os.Stdout); err != nil {
					fmt.Println("Unable to generate bash completion", err)
					os.Exit(1)
				}
			case "zsh":
				if err := c.Parent().GenZshCompletion(os.Stdout); err != nil {
					fmt.Println("Unable to generate zsh completion", err)
					os.Exit(1)
				}
			default:
				fmt.Println("Unsupported shell", a[0])
			}
		},
	}
	return cmd
}
