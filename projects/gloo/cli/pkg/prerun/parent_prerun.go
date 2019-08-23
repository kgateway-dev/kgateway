package prerun

import "github.com/spf13/cobra"

func CallParentPrerun(cmd *cobra.Command, args []string) error {
	if parent := cmd.Parent(); parent != nil {
		if parent.PersistentPreRunE != nil {
			if err := parent.PersistentPreRunE(parent, args); err != nil {
				return err
			}
		}
		return CallParentPrerun(parent, args)
	}
	return nil
}
