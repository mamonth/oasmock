## Context

The OASMock CLI currently uses viper to bind command-line flags and environment variables (prefixed with `OASMOCK_`). Configuration precedence is: flags > environment variables > defaults. The codebase already imports `github.com/spf13/viper`, but config file support is not enabled. Users need a persistent configuration option that can be committed to version control, reducing repetitive command-line arguments and environment variable setup.

## Goals / Non-Goals

**Goals:**
- Add support for a `.oasmock.yaml` configuration file in the current working directory (and optionally the user's home directory)
- Define YAML structure that mirrors existing CLI flag names (kebab-case)
- Implement configuration precedence: CLI flags > environment variables > YAML config > defaults
- Maintain backward compatibility: existing environment variable and CLI flag behavior unchanged
- Support all currently available options with simplified schema configuration: `schema` (single), `schemas` (list with optional `src` and `prefix`), `port`, `delay`, `verbose`, `nocors`, `history-size`, `no-control-api`

**Non-Goals:**
- Support for other configuration file formats (JSON, TOML, etc.)
- Support for nested configuration directories (e.g., `.oasmock/`)
- Dynamic reloading of configuration file while server is running
- Environment variable overrides for array options (`from` and `prefix`)

## Decisions

**1. Use viper's built‑in config file support**
   - Viper is already a dependency and provides consistent precedence handling (flag > env > config > default)
   - Adding config file support requires minimal changes: set config name/path and call `ReadInConfig()`
   - Rationale: avoids introducing new dependencies and reuses existing configuration infrastructure.

**2. Config file location and naming**
   - Primary location: current working directory (`.oasmock.yaml`)
   - Fallback location: user's home directory (`~/.oasmock.yaml`)
   - Rationale: common pattern for CLI tools; allows project‑specific config and personal defaults.

**3. YAML key naming**
   - Use simplified schema configuration keys: `schema` (single string) and `schemas` (list of strings or objects with `src` and optional `prefix`)
   - For other options, use kebab‑case keys matching the flag names: `port`, `delay`, `verbose`, `nocors`, `history-size`, `no-control-api`
   - Viper automatically normalizes `-` to `_` when binding environment variables, so the same keys work across all sources.
   - Rationale: simplified schema configuration improves readability while maintaining consistency for other options.

**4. Schema configuration mapping**
   - Support two YAML formats: `schema: path` for single schema, and `schemas:` list for multiple schemas.
   - For `schemas:` list, each element can be a string (schema path) or object with `src:` and optional `prefix:`.
   - Map these to internal `from` and `prefix` arrays after reading viper config.
   - Rationale: simplified configuration format improves usability while maintaining compatibility with existing CLI array logic.

**5. CLI flag precedence for schema configuration**
   - Bind `--from` and `--prefix` flags to viper keys `from` and `prefix` for backward compatibility.
   - If `--from` flag is provided (via CLI), ignore YAML `schema`/`schemas` configuration entirely (CLI flags override config file).
   - Rationale: maintain existing CLI behavior while allowing config file as default.

**6. Schema configuration validation**
   - Validate that `schema` and `schemas` are mutually exclusive (both cannot be present in the same config file).
   - Validate that `schemas` list elements are properly formed (strings or objects with `src`).
   - Rationale: prevents ambiguous configuration and provides clear error messages.

**7. Error handling for missing config file**
   - Call `viper.ReadInConfig()` early (e.g., in `init()` or at the start of `runMock`)
   - If the config file is not found, treat it as a non‑error and continue with other sources.
   - Rationale: config file is optional; users may rely solely on flags/env.

**8. Precedence verification**
   - Write integration tests that verify the correct order: flag overrides environment variable, environment variable overrides config file.
   - Rationale: ensures the implemented behavior matches the specification.

## Risks / Trade-offs

**Risk: Schema format mapping complexity**
   - Mitigation: Add unit tests for parsing both `schema` and `schemas` formats, mapping to internal `from`/`prefix` arrays.

**Risk: Silent failures on malformed YAML**
   - Mitigation: Log a warning when the config file exists but cannot be parsed, and continue with other sources.

**Trade-off: No environment variable support for schema arrays**
   - Environment variables for schema arrays are complex (comma‑separated lists, escaping). Since config files now provide a better alternative, we keep the current behavior (no env binding for schema arrays).
   - Acceptable because schema configuration is more naturally expressed in YAML.

**Trade-off: Single config file location**
   - Only searching the current directory (and optionally home) simplifies the implementation. Users who need different configs can change the working directory or use flags.
   - If needed later, we can add an `--config` flag to specify a custom path.