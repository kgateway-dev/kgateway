package prerun

import "github.com/spf13/cobra"

// the purpose of this function is to manually run PersistenPreRunE of parent
// CLI commands. this allows PersistenPreRunE to be defined on multiple nodes
// in the command tree
func CallParentPrerun(cmd *cobra.Command, args []string) error {
	// if the executing subcommand has no PersistentPreRunE,
	// the most recent parent.PersistentPreRunE will execute.
	// therefore, we should skip the first parent's PersistentPreRunE
	// as it is the currently executing PersistentPreRunE
	skipFirstPreRun := cmd.PersistentPreRunE == nil

	for parent := cmd.Parent(); parent != nil; parent = parent.Parent() {
		if parent.PersistentPreRunE != nil {
			if skipFirstPreRun {
				skipFirstPreRun = false
				continue
			}
			if err := parent.PersistentPreRunE(parent, args); err != nil {
				return err
			}
		}
	}

	return nil
}
