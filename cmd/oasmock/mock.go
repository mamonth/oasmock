package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	defaultPort  = 19191
	defaultDelay = 100 // milliseconds
	minPort      = 1
	maxPort      = 65535
)

type mockConfig struct {
	sources      []string
	prefixes     []string
	port         int
	delay        int
	verbose      bool
	noCORS       bool
	historySize  int
	noControlAPI bool
}

func validationError(format string, args ...any) error {
	return &cliError{
		code:    2,
		message: fmt.Sprintf(format, args...),
	}
}

func schemaError(format string, args ...any) error {
	return &cliError{
		code:    3,
		message: fmt.Sprintf(format, args...),
	}
}

func portError(format string, args ...any) error {
	return &cliError{
		code:    4,
		message: fmt.Sprintf(format, args...),
	}
}

func parseSchemaConfig(cmd *cobra.Command) error {
	// If --from flag was provided, ignore YAML schema configuration
	if cmd != nil && cmd.Flags().Changed("from") {
		return nil
	}

	schemaVal := viper.Get("schema")
	schemasVal := viper.Get("schemas")

	// Check mutual exclusivity
	if schemaVal != nil && schemasVal != nil {
		return validationError("cannot specify both 'schema' and 'schemas' in config file")
	}

	// Handle single schema
	if schemaVal != nil {
		schema, ok := schemaVal.(string)
		if !ok {
			return validationError("'schema' must be a string")
		}
		config.sources = []string{schema}
		config.prefixes = []string{}
		return nil
	}

	// Handle schemas list
	if schemasVal != nil {
		schemas, ok := schemasVal.([]any)
		if !ok {
			return validationError("'schemas' must be a list")
		}

		var sources []string
		var prefixes []string

		for i, item := range schemas {
			switch v := item.(type) {
			case string:
				sources = append(sources, v)
				prefixes = append(prefixes, "")
			case map[string]any:
				src, ok := v["src"].(string)
				if !ok {
					return validationError("schemas[%d] must have a string 'src' field", i)
				}
				sources = append(sources, src)

				prefix, _ := v["prefix"].(string)
				prefixes = append(prefixes, prefix)
			default:
				return validationError("schemas[%d] must be a string or object with 'src' field", i)
			}
		}

		config.sources = sources
		config.prefixes = prefixes
	}

	return nil
}

var config mockConfig

var mockCmd = &cobra.Command{
	Use:           "mock",
	Short:         "Start the mock server",
	Long:          `Start an HTTP server that mocks endpoints defined in OpenAPI schema(s).`,
	RunE:          runMock,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.AddCommand(mockCmd)
	// Set mock as default command when no subcommand given
	rootCmd.Run = mockCmd.Run
	rootCmd.RunE = mockCmd.RunE

	// Ensure errors and usage are handled by our custom logic
	mockCmd.SilenceErrors = true
	mockCmd.SilenceUsage = true

	// Define flags
	mockCmd.Flags().StringArrayVar(&config.sources, "from", []string{"src/openapi.yaml"}, "Source OpenAPI schema(s). Can be specified multiple times.")
	mockCmd.Flags().StringArrayVar(&config.prefixes, "prefix", []string{}, "URI prefix for each schema. Should be specified for each --from parameter.")
	mockCmd.Flags().IntVar(&config.port, "port", defaultPort, "Port to listen on.")
	mockCmd.Flags().IntVar(&config.delay, "delay", defaultDelay, "Delay between request and response in milliseconds.")
	mockCmd.Flags().BoolVar(&config.verbose, "verbose", false, "Enable verbose logging.")
	mockCmd.Flags().BoolVar(&config.noCORS, "nocors", false, "Disable automatic CORS compliance.")
	mockCmd.Flags().IntVar(&config.historySize, "history-size", server.DefaultHistorySize, "Maximum number of requests to keep in history.")
	mockCmd.Flags().BoolVar(&config.noControlAPI, "no-control-api", false, "Disable the management control API.")

	// Bind environment variables
	viper.SetEnvPrefix("OASMOCK")
	viper.AutomaticEnv()

	// Configure config file
	viper.SetConfigName(".oasmock")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME")
	viper.SetConfigType("yaml")
	_ = viper.BindPFlag("port", mockCmd.Flags().Lookup("port"))
	_ = viper.BindPFlag("delay", mockCmd.Flags().Lookup("delay"))
	_ = viper.BindPFlag("verbose", mockCmd.Flags().Lookup("verbose"))
	_ = viper.BindPFlag("nocors", mockCmd.Flags().Lookup("nocors"))
	_ = viper.BindPFlag("history_size", mockCmd.Flags().Lookup("history-size"))
	_ = viper.BindPFlag("no_control_api", mockCmd.Flags().Lookup("no-control-api"))
	_ = viper.BindPFlag("from", mockCmd.Flags().Lookup("from"))
	_ = viper.BindPFlag("prefix", mockCmd.Flags().Lookup("prefix"))
}

func runMock(cmd *cobra.Command, args []string) error {
	// Read config file (if present)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Config file exists but is malformed - log warning
			slog.Warn("Failed to read config file", "err", err)
		}
		// Config file not found is not an error
	}

	// Parse schema configuration from YAML (if present)
	if err := parseSchemaConfig(cmd); err != nil {
		return err
	}

	// Read values from viper (environment overrides)
	port := viper.GetInt("port")
	delay := viper.GetInt("delay")
	verbose := viper.GetBool("verbose")
	noCORS := viper.GetBool("nocors")
	historySize := viper.GetInt("history_size")
	noControlAPI := viper.GetBool("no_control_api")

	// Configure structured logging
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))

	// Validate flag combinations
	if len(config.sources) != len(config.prefixes) && len(config.prefixes) != 0 {
		return validationError("number of --prefix flags must match number of --from flags, or no --prefix flags provided")
	}
	if port <= 0 || port > maxPort {
		return validationError("port must be between 1 and 65535")
	}
	if delay < 0 {
		return validationError("delay cannot be negative")
	}
	if historySize < 0 {
		return validationError("history size cannot be negative")
	}

	// Load OpenAPI schemas
	schemas, err := loader.LoadSchemas(config.sources, config.prefixes)
	if err != nil {
		return schemaError("failed to load schemas: %v", err)
	}

	// Prepare server configuration
	serverConfig := server.Config{
		Port:             port,
		Delay:            time.Duration(delay) * time.Millisecond,
		Verbose:          verbose,
		EnableCORS:       !noCORS,
		HistorySize:      historySize,
		EnableControlAPI: !noControlAPI,
	}

	// Create and start server
	srv, err := server.New(serverConfig, schemas)
	if err != nil {
		return schemaError("failed to create server: %v", err)
	}

	// Start server in a goroutine so we can handle signals
	serverErrChan := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "err", err)
			serverErrChan <- err
		}
	}()

	// Wait for interrupt signal
	slog.Info("Mock server started", "port", port)
	slog.Info("Press Ctrl+C to stop")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for either signal or server error
	select {
	case sig := <-sigChan:
		slog.Info("Received signal, shutting down gracefully", "signal", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Graceful shutdown failed", "err", err)
			return fmt.Errorf("shutdown failed: %w", err)
		}
		slog.Info("Server stopped gracefully")
		return nil
	case err := <-serverErrChan:
		return fmt.Errorf("server error: %w", err)
	}
}
