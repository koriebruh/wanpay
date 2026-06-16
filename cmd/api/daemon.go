package main

import (
	"fmt"
	"syscall"

	daemon "github.com/sevlyar/go-daemon"
	"github.com/spf13/cobra"

	"wanpey/core/internal/app"
)

const (
	// Relative to WorkDir ("./") so the daemon user does not need write access
	// to world-writable directories like /tmp (symlink attack vector).
	pidFile = "wanpey.pid"
	logFile = "wanpey-daemon.log"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the daemon process",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Run the server as a background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := &daemon.Context{
			PidFileName: pidFile,
			PidFilePerm: 0644,
			LogFileName: logFile,
			LogFilePerm: 0640,
			WorkDir:     "./",
			Umask:       027,
		}

		d, err := ctx.Reborn()
		if err != nil {
			return fmt.Errorf("daemon start failed: %w", err)
		}

		// Non-nil d means this is the parent process — it's done.
		if d != nil {
			fmt.Println("wanpey daemon started, PID:", d.Pid)
			return nil
		}

		defer func() {
			if err := ctx.Release(); err != nil {
				fmt.Printf("failed to release daemon context: %v\n", err)
			}
		}()

		a := app.New()
		if err := a.Boot(); err != nil {
			return err
		}
		return a.Run()
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon process",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := &daemon.Context{PidFileName: pidFile}

		d, err := ctx.Search()
		if err != nil {
			return fmt.Errorf("daemon not found: %w", err)
		}

		if err := d.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to stop daemon: %w", err)
		}

		fmt.Println("stop signal sent to daemon PID:", d.Pid)
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := &daemon.Context{PidFileName: pidFile}

		d, err := ctx.Search()
		if err != nil {
			fmt.Println("daemon is NOT running")
			return nil
		}

		fmt.Printf("daemon is RUNNING (PID: %d)\n", d.Pid)
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
}
