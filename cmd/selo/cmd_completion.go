package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for selo.

To load completions:

Bash:
  $ source <(selo completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ selo completion bash > /etc/bash_completion.d/selo
  # macOS:
  $ selo completion bash > $(brew --prefix)/etc/bash_completion.d/selo

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session, execute once:
  $ selo completion zsh > "${fpath[1]}/_selo"
  # You will need to start a new shell for this setup to take effect.

Fish:
  $ selo completion fish | source
  # To load completions for each session, execute once:
  $ selo completion fish > ~/.config/fish/completions/selo.fish

PowerShell:
  PS> selo completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> selo completion powershell > selo.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE:                  runCompletionCmd,
}

func runCompletionCmd(cmd *cobra.Command, args []string) error {
	switch args[0] {
	case "bash":
		return rootCmd.GenBashCompletionV2(os.Stdout, true)
	case "zsh":
		return rootCmd.GenZshCompletion(os.Stdout)
	case "fish":
		return rootCmd.GenFishCompletion(os.Stdout, true)
	case "powershell":
		return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
	default:
		return fmt.Errorf("unsupported shell: %s", args[0])
	}
}
