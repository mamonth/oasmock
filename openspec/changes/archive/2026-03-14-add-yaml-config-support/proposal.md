## Why

Provide a persistent configuration option via YAML file for users who want to store configuration alongside project, reducing need for environment variables or long CLI arguments. This improves developer experience by allowing project-specific configuration to be committed to version control.

## What Changes

- Add support for `.oasmock.yaml` configuration file in current working directory (or user home directory)
- Define YAML structure with simplified schema configuration (single `schema:` key for one schema, `schemas:` list for multiple with optional `src` and `prefix`), plus other CLI options (port, verbose, nocors, delay, no-control-api)
- Implement configuration precedence: CLI arguments > environment variables > YAML config > defaults
- Update CLI documentation to include YAML config file specification
- Ensure backward compatibility: existing environment variable and CLI argument behavior unchanged

## Capabilities

### New Capabilities
<!-- Capabilities being introduced. Replace <name> with kebab-case identifier (e.g., user-auth, data-export, api-rate-limiting). Each creates specs/<name>/spec.md -->
- `yaml-config`: Support for YAML configuration file format and precedence rules

### Modified Capabilities
<!-- Existing capabilities whose REQUIREMENTS are changing (not just implementation).
     Only list here if spec-level behavior changes. Each needs a delta spec file.
     Use existing spec names from openspec/specs/. Leave empty if no requirement changes. -->
- `cli`: Add requirement for YAML configuration file support and update configuration precedence

## Impact

- CLI parsing logic will need to read and merge config file, with special handling for simplified schema configuration format
- Configuration precedence logic updated in configuration layer
- Documentation updates in docs/cli.md
- Potential addition of a new package for YAML parsing (if not already present)