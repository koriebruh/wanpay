package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wanpey",
	Short: "Wanpey — payment gateway aggregator",
}

func main() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(migrateCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
