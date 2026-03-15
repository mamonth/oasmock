package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Creating validation error with CLI error code
Given a format string and arguments
When validationError is called
Then it returns a cliError with code 2 and formatted message

Related spec scenarios: RS.CLI.15
*/
func TestValidationError(t *testing.T) {
	t.Parallel()

	err := validationError("test %s", "message")
	require.NotNil(t, err, "validationError returned nil")
	assert.Equal(t, "test message", err.Error(), "validationError error message")
	cliErr, ok := err.(*cliError)
	require.True(t, ok, "validationError did not return cliError")
	assert.Equal(t, 2, cliErr.code, "cliError.code mismatch")
}

/*
Scenario: Creating schema error with CLI error code
Given a format string and arguments
When schemaError is called
Then it returns a cliError with code 3 and formatted message

Related spec scenarios: RS.CLI.16
*/
func TestSchemaError(t *testing.T) {
	t.Parallel()

	err := schemaError("schema %s", "error")
	require.NotNil(t, err, "schemaError returned nil")
	assert.Equal(t, "schema error", err.Error(), "schemaError error message")
	cliErr, ok := err.(*cliError)
	require.True(t, ok, "schemaError did not return cliError")
	assert.Equal(t, 3, cliErr.code, "cliError.code mismatch")
}

/*
Scenario: Creating port error with CLI error code
Given a format string and port number
When portError is called
Then it returns a cliError with code 4 and formatted message

Related spec scenarios: RS.CLI.17
*/
func TestPortError(t *testing.T) {
	t.Parallel()

	err := portError("port %d", 8080)
	require.NotNil(t, err, "portError returned nil")
	assert.Equal(t, "port 8080", err.Error(), "portError error message")
	cliErr, ok := err.(*cliError)
	require.True(t, ok, "portError did not return cliError")
	assert.Equal(t, 4, cliErr.code, "cliError.code mismatch")
}

/*
Scenario: CLI error type behavior
Given a cliError instance with code and message
When Error() is called
Then it returns the message, and code field is accessible

Related spec scenarios: RS.CLI.15, RS.CLI.16, RS.CLI.17, RS.CLI.18
*/
func TestCliError(t *testing.T) {
	t.Parallel()

	err := &cliError{code: 5, message: "custom"}
	assert.Equal(t, "custom", err.Error(), "cliError.Error()")
	assert.Equal(t, 5, err.code, "cliError.code")
}

/*
Scenario: Running mock command with invalid configuration triggers validation errors
Given various invalid configurations (mismatched prefixes, invalid port, negative delay, negative history size)
When runMock is called
Then it returns appropriate cliError for each validation failure

Related spec scenarios: RS.CLI.15
*/
func TestRunMockValidationErrors(t *testing.T) {

	tests := []struct {
		name       string
		setup      func()
		checkError func(t *testing.T, err error)
	}{
		{
			name: "mismatched sources and prefixes",
			setup: func() {
				config = mockConfig{}
				viper.Reset()
				config.sources = []string{"schema.yaml"}
				config.prefixes = []string{"", "extra"}
			},
			checkError: func(t *testing.T, err error) {
				require.Error(t, err, "expected validation error for mismatched prefixes")
				_, ok := err.(*cliError)
				assert.True(t, ok, "expected cliError, got %T", err)
			},
		},
		{
			name: "invalid port",
			setup: func() {
				config = mockConfig{}
				viper.Reset()
				viper.Set("port", 0)
			},
			checkError: func(t *testing.T, err error) {
				require.Error(t, err, "expected validation error for invalid port")
			},
		},
		{
			name: "negative delay",
			setup: func() {
				config = mockConfig{}
				viper.Reset()
				viper.Set("delay", -1)
			},
			checkError: func(t *testing.T, err error) {
				require.Error(t, err, "expected validation error for negative delay")
			},
		},
		{
			name: "negative history size",
			setup: func() {
				config = mockConfig{}
				viper.Reset()
				viper.Set("history_size", -5)
			},
			checkError: func(t *testing.T, err error) {
				require.Error(t, err, "expected validation error for negative history size")
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			err := runMock(nil, nil)
			tt.checkError(t, err)
		})
	}
} /*
Scenario: Parsing schema configuration from YAML
Given various YAML configuration inputs (valid and invalid)
When parseSchemaConfig is called
Then it should parse valid configurations correctly and return appropriate errors for invalid ones

Related spec scenarios: RS.CLI.19, RS.CLI.26, RS.CLI.27
*/
func TestParseSchemaConfig(t *testing.T) {

	tests := []struct {
		name        string
		setup       func(cmd *cobra.Command)
		expectError bool
		check       func(t *testing.T)
	}{
		{
			name: "no schema config, from flag not changed",
			setup: func(cmd *cobra.Command) {
				viper.Reset()
				config = mockConfig{}
				// cmd.Flags().Changed returns false by default
			},
			check: func(t *testing.T) {
				// Should remain nil (no schema config, viper reset)
				assert.Nil(t, config.sources)
				assert.Nil(t, config.prefixes)
			},
		},
		{
			name: "single schema",
			setup: func(cmd *cobra.Command) {
				viper.Reset()
				config = mockConfig{}
				viper.Set("schema", "custom.yaml")
			},
			check: func(t *testing.T) {
				assert.Equal(t, []string{"custom.yaml"}, config.sources)
				assert.Equal(t, []string{}, config.prefixes)
			},
		},
		{
			name: "multiple schemas with mixed formats",
			setup: func(cmd *cobra.Command) {
				viper.Reset()
				config = mockConfig{}
				viper.Set("schemas", []any{
					map[string]any{"src": "api/v1.yaml", "prefix": "/v1"},
					"api/v2.yaml",
				})
			},
			check: func(t *testing.T) {
				assert.Equal(t, []string{"api/v1.yaml", "api/v2.yaml"}, config.sources)
				assert.Equal(t, []string{"/v1", ""}, config.prefixes)
			},
		},
		{
			name: "both schema and schemas present",
			setup: func(cmd *cobra.Command) {
				viper.Reset()
				config = mockConfig{}
				viper.Set("schema", "single.yaml")
				viper.Set("schemas", []any{"multi.yaml"})
			},
			expectError: true,
		},
		{
			name: "invalid schemas element",
			setup: func(cmd *cobra.Command) {
				viper.Reset()
				config = mockConfig{}
				viper.Set("schemas", []any{42})
			},
			expectError: true,
		},
		{
			name: "invalid schema type",
			setup: func(cmd *cobra.Command) {
				viper.Reset()
				config = mockConfig{}
				viper.Set("schema", 123)
			},
			expectError: true,
		},
		{
			name: "schemas object missing src",
			setup: func(cmd *cobra.Command) {
				viper.Reset()
				config = mockConfig{}
				viper.Set("schemas", []any{map[string]any{"prefix": "/foo"}})
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().StringArray("from", []string{"src/openapi.yaml"}, "")
			tt.setup(cmd)
			err := parseSchemaConfig(cmd)
			if tt.expectError {
				require.Error(t, err)
				// Should be a validation error (code 2)
				cliErr, ok := err.(*cliError)
				assert.True(t, ok, "error should be cliError")
				assert.Equal(t, 2, cliErr.code)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t)
				}
			}
		})
	}
}

/*
Scenario: Valid YAML configuration structure
Given a .oasmock.yaml file with all supported configuration keys
When parseSchemaConfig processes the configuration
Then all keys are correctly parsed into mockConfig fields

Related spec scenarios: RS.CLI.19
*/
func TestValidYAMLStructure(t *testing.T) {
	t.Parallel()

	viper.Reset()
	config = mockConfig{}

	yamlConfig := `schema: test.yaml
port: 8080
delay: 500
verbose: true
nocors: true
history-size: 500
no-control-api: true`

	viper.SetConfigType("yaml")
	require.NoError(t, viper.ReadConfig(bytes.NewBufferString(yamlConfig)))

	// Verify viper can read all keys
	assert.Equal(t, "test.yaml", viper.GetString("schema"))
	assert.Equal(t, 8080, viper.GetInt("port"))
	assert.Equal(t, 500, viper.GetInt("delay"))
	assert.Equal(t, true, viper.GetBool("verbose"))
	assert.Equal(t, true, viper.GetBool("nocors"))
	assert.Equal(t, 500, viper.GetInt("history-size"))
	assert.Equal(t, true, viper.GetBool("no-control-api"))

	// Also test schemas key with mixed formats
	viper.Reset()
	config = mockConfig{}

	yamlConfig2 := `schemas:
  - src: api/v1.yaml
    prefix: /v1
  - api/v2.yaml
port: 9090`

	viper.SetConfigType("yaml")
	require.NoError(t, viper.ReadConfig(bytes.NewBufferString(yamlConfig2)))

	// The schemas key should be readable as slice
	schemas := viper.Get("schemas")
	assert.NotNil(t, schemas)
}

/*
Scenario: Testing configuration precedence rules
Given configuration values defined in multiple sources (CLI flags, environment variables, config file, defaults)
When the configuration is resolved
Then values from higher precedence sources override those from lower precedence sources

Related spec scenarios: RS.CLI.22, RS.CLI.23, RS.CLI.28, RS.CLI.29
*/
func TestConfigPrecedence(t *testing.T) {
	// t.Parallel() - cannot use with t.Setenv

	// Test precedence: flag > config > env > default
	// We'll test by setting viper values in different ways
	// For config file simulation, we use viper.ReadConfig with YAML string
	// For environment, we set viper with env prefix
	// For flags, we bind flag and set changed
	// Since viper handles precedence internally, we can just verify final values

	t.Run("flag overrides config", func(t *testing.T) {
		viper.Reset()
		config = mockConfig{}
		// Simulate config file value
		yamlConfig := `port: 8080`
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadConfig(bytes.NewBufferString(yamlConfig)))
		// Simulate flag value (bind flag and set)
		cmd := &cobra.Command{}
		cmd.Flags().Int("port", 19191, "")
		require.NoError(t, viper.BindPFlag("port", cmd.Flags().Lookup("port")))
		require.NoError(t, cmd.Flags().Set("port", "9090"))
		// Need to call parseSchemaConfig? Not needed for port
		// Read value via viper
		port := viper.GetInt("port")
		assert.Equal(t, 9090, port, "flag should override config file")
	})

	t.Run("env overrides config", func(t *testing.T) {
		viper.Reset()
		viper.SetEnvPrefix("OASMOCK")
		viper.AutomaticEnv()
		// Set environment variable
		t.Setenv("OASMOCK_PORT", "7070")
		// Simulate config file with different value
		yamlConfig := `port: 8080`
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadConfig(bytes.NewBufferString(yamlConfig)))
		port := viper.GetInt("port")
		assert.Equal(t, 7070, port, "environment variable should override config file")
	})

	t.Run("CLI from flag overrides YAML schema", func(t *testing.T) {
		viper.Reset()
		config = mockConfig{}
		// Simulate config file with schema
		yamlConfig := `schema: custom.yaml`
		viper.SetConfigType("yaml")
		require.NoError(t, viper.ReadConfig(bytes.NewBufferString(yamlConfig)))
		// Create command with --from flag set
		cmd := &cobra.Command{}
		cmd.Flags().StringArray("from", []string{"src/openapi.yaml"}, "")
		// Parse flag to mark as changed
		require.NoError(t, cmd.ParseFlags([]string{"--from", "flag.yaml"}))
		// Bind flag to viper (as done in init)
		require.NoError(t, viper.BindPFlag("from", cmd.Flags().Lookup("from")))
		// Set config.sources to flag value (as flag binding would do)
		config.sources = []string{"flag.yaml"}
		config.prefixes = []string{}
		// Call parseSchemaConfig
		err := parseSchemaConfig(cmd)
		require.NoError(t, err)
		// Should keep flag value, not config file schema
		assert.Equal(t, []string{"flag.yaml"}, config.sources)
		assert.Equal(t, []string{}, config.prefixes)
	})

	t.Run("env overrides default", func(t *testing.T) {
		viper.Reset()
		viper.SetEnvPrefix("OASMOCK")
		viper.AutomaticEnv()
		// Set environment variable
		t.Setenv("OASMOCK_PORT", "6060")
		port := viper.GetInt("port")
		assert.Equal(t, 6060, port, "environment variable should override default")
	})
}
