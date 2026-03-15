package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	// Set default slog handler to stderr with info level
	// This will be overridden by verbose flag in mock command
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
}

type cliError struct {
	code    int
	message string
}

func (e *cliError) Error() string {
	return e.message
}

var version = "dev" // will be set by linker flags during release builds

var rootCmd = &cobra.Command{
	Use:           "oasmock",
	Short:         "OpenAPI-based mock server",
	Long:          `OpenSpec mock tool: a high-performance mock server for OpenAPI schemas with extensions.`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.PersistentFlags().BoolP("version", "v", false, "Show version information")
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if versionFlag, _ := cmd.Flags().GetBool("version"); versionFlag {
			slog.Info("oasmock version " + version)
			os.Exit(0)
		}
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error(err.Error())
		// Determine exit code
		code := 1
		if cliErr, ok := err.(*cliError); ok {
			code = cliErr.code
		}
		os.Exit(code)
	}
}
