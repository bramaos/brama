package cli

import "github.com/spf13/cobra"

// newAnonymizeCmd groups the commands that own the anonymize block.
//
// `init` writes it, `review` amends it, and `check` reads it. `review` is still to
// come, which is why `init` points at it and cannot yet be followed there.
func newAnonymizeCmd(env *console) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "anonymize",
		Short: "Decide and check what happens to each column's values",
	}
	cmd.AddCommand(newAnonymizeInitCmd(env), newAnonymizeCheckCmd(env))
	return cmd
}
