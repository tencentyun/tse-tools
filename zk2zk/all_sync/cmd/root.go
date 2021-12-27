package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:     "all_sync",
	Version: "1.0.0",
	Short:   "all_sync is a tool for synchronizing zookeeper all node.",
	Long: "all_sync is a tool for synchronizing zookeeper all node. " +
		"The tool will mock on the destination side as a persistent node " +
		"when ephemeral nodes changed on the source side. " +
		"Also, the tool will synchronize the persistent node from source side to destination side directly. ",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("execute fail, ", err)
		os.Exit(-1)
	}
}
