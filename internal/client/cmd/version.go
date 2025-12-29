package cmd

import (
	"fmt"

	"tunnox-core/internal/version"

	"github.com/spf13/cobra"
)

// versionCmd 显示版本信息
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long: `Show detailed version information including build time and git commit.

Example:
  tunnox version`,
	Run: runVersion,
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Println()
	fmt.Printf("Tunnox Client %s\n", version.GetVersion())
	fmt.Println()
	fmt.Println("A high-performance enterprise-grade tunneling platform")
	fmt.Println("supporting TCP, WebSocket, KCP, and QUIC protocols.")
	fmt.Println()
	fmt.Println("For more information, visit: https://tunnox.net")
	fmt.Println()
}
