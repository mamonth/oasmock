## 1. Configuration file reading

- [x] 1.1 Add viper config file setup in mock.go init() (set config name, add search paths)
- [x] 1.2 Bind CLI flags to viper using `BindPFlag` (port, delay, verbose, nocors, history-size, no-control-api, from, prefix)
- [x] 1.3 Call `viper.ReadInConfig()` early in `runMock` or `init`, handling missing config file gracefully
- [x] 1.4 Implement schema configuration mapping: read `schema` or `schemas` from viper, convert to internal `from` and `prefix` arrays
- [x] 1.5 Validate schema configuration (mutual exclusivity of `schema` and `schemas`, proper structure of `schemas` list)

## 2. YAML structure validation

- [x] 2.1 Define YAML example in docs/cli.md (new section "Configuration File") with simplified schema format examples
- [x] 2.2 Ensure viper key mappings match kebab‑case for standard options, and handle `schema`/`schemas` keys appropriately

## 3. Testing

- [x] 3.1 Write unit tests for config file parsing (valid YAML, malformed YAML, missing file)
- [x] 3.2 Write integration tests for precedence scenarios (CLI flag > environment variable > config file)
- [x] 3.3 Update existing CLI tests to accommodate config file (ensure no regressions)

## 4. Documentation

- [x] 4.1 Update docs/cli.md with config file section describing location, format, and precedence
- [x] 4.2 Add examples of `.oasmock.yaml` with single schema, multiple schemas (with and without prefixes), and all options