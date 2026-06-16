package main

import (
	"github.com/spf13/cobra"

	"wanpey/core/internal/app"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the server in the foreground",
	RunE: func(cmd *cobra.Command, args []string) error {
		a := app.New()
		if err := a.Boot(); err != nil {
			return err
		}
		return a.Run()
	},
}
