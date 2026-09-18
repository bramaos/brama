package cli

import "github.com/spf13/cobra"

// newAnonymizeCmd groups the commands that own the anonymize block.
//
// `init` writes it, `review` amends it, and `check` reads it. Only `check` exists so
// far, and it is the one that writes nothing at all.
func newAnonymizeCmd(env *console) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "anonymize",
		Short: "Decide and check what happens to each column's values",
	}
	cmd.AddCommand(newAnonymizeCheckCmd(env))
	return cmd
}
