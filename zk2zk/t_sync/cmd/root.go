package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var rootCmd = &cobra.Command{
	Use:     "t_sync",
	Version: "1.0.0",
	Short:   "t_sync is a tool for synchronizing zookeeper ephemeral node.",
	Long:    "t_sync is a tool for synchronizing zookeeper ephemeral node. " +
		"The tool will mock on the destination side as a persistent node " +
		"when ephemeral nodes changed on the source side.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("execute fail, ", err)
		os.Exit(-1)
	}
}
