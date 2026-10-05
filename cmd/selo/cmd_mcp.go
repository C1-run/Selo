package main

import (
	"os"

	"github.com/C1-run/selo/internal/mcpserver"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Expose Selo's safety checks over the Model Context Protocol",
	Long: `Run Selo as an MCP server on stdio so coding agents (OpenCode, and any
MCP client) can call the real checks — forbidden files, allowlist, claims,
secrets, limits — instead of a reimplemented subset.`,
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve the checks over stdio JSON-RPC (MCP)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcpserver.New(os.Stdin, os.Stdout).Serve()
	},
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
}
