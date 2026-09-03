# OASMock

**Agent SHOULD NOT change this file, only suggest changes to user when inconsistency or potential improvement can be done**

## References
- [Project-specific standards](docs/project.md)
- [Architecture docs](docs/architecture.md)
- [Project specs (BDD)](openspec/specs/)

## Development Guidelines
- Always read project standards and architecture when new session started
- Cognitive Complexity ([metric by Sonar Source](https://redirect.sonarsource.com/doc/cognitive-complexity.html)) MUST be as low as possible by keeping conditionals simple and nesting levels moderately low (with helper functions and/or declarative approach)
- Module coupling MUST be moderately low to enable clean unit testing and make codebase resilient to changes
- Module cohesion (module context, knowledge and logic density) MUST be as high as possible 
- Code duplication SHOULD be as minimal as possible as long as it reduces complexity (see the rule about coupling and cohesion)
- Function length SHOULD be ignored, as long as no code or logic duplication is present and code responsibility is in the right place (high cohesion)
- Data-driven approaches SHOULD be used instead of repetitive control structures (declarative over imperative)
- Core constants or configuration MUST be defined in one place, and derived representations (e.g., a set for fast lookup) SHOULD be derived programmatically.
- When in need to perform frequent membership checks, source-of-truth slice SHOULD be converted into a map (set) once—preferably at initialization (init).

## Code Design 
- Use design-first and TDD principle:
  1. Design function interface according to usage need and check it's usability in context
  2. Write or edit tests for parent code (code where new interface is used), mocking new/edited interface, to ensure host code works as expected
  3. Write or edit tests for interface itself
  4. Write implementation of interface until tests will pass

## Quality Assurance Guidelines
- All tests MUST follow common development guidelines
- All test functions MUST contain multiline (/**/) comment before function declaration with:
  - Gherkin notation of test case
  - List of related requirement scenario codes from openspec/specs at separate line

    example: 
    ```
    /*
    Scenario: Adding records to ring buffer and retrieving all
    Given a ring buffer with capacity 3
    When records are added up to and beyond capacity
    Then GetAll returns correct records, oldest records are overwritten on overflow
    
    Related spec scenarios: RS.CLI.2, RS.CLI.3
    */
    ```
- Use parameterized tests when the all test’s steps (AAA) are identical across all cases, and only the input and expected output differ. Otherwise, write separate tests.
- Unit tests and integration tests placement and conventions are defined in [project standards](docs/project.md#testing-standarts)

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **oasmock** (2518 symbols, 5666 relationships, 133 execution flows).

> Index stale? Run `node .gitnexus/run.cjs analyze --index-only` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? Bootstrap with `npx`, `bunx`, or `pnpm dlx` — e.g. `bunx gitnexus@latest analyze` (npm 11 npx crash; #1939).

## Always Do

- **MUST run impact analysis before editing.** Use `impact({target: "symbolName", direction: "upstream"})` (MCP) or `node .gitnexus/run.cjs impact "symbolName" --direction upstream --repo .` (CLI fallback); report callers, processes, and risk. Never substitute grep for graph analysis.
- **MUST analyze graph changes before committing.** Use `detect_changes({scope: "all"})` (MCP) or `node .gitnexus/run.cjs detect-changes --scope all --repo .` (CLI fallback). `partial: true` or `truncated: true` is not a clean check — a zero means unseen, not unaffected; re-run it. For regression review: `detect_changes({scope: "compare", base_ref: "main"})` or `node .gitnexus/run.cjs detect-changes --scope compare --base-ref "main" --repo .`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- **MUST treat `risk: UNKNOWN` as unresolved, not as low.** An empty caller set is not evidence the symbol is unused — it can also mean the callers are not resolvable by the index (plain-object property access, dynamic dispatch, cross-language calls). `impact` pairs `UNKNOWN` with a `riskNote` saying so. Confirm with a text search before treating the symbol as safe to change or delete; do not proceed on the strength of a zero.
- When exploring unfamiliar code, use `query({search_query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method before MCP/CLI impact analysis.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis, and never read `UNKNOWN` as an all-clear — it means the walk could not answer, which is the one verdict that requires confirming by other means.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit before MCP/CLI graph change analysis.

## Resources

| Resource | Use for |
| --- | --- |
| `gitnexus://repo/oasmock/context` | Codebase overview, check index freshness |
| `gitnexus://repo/oasmock/clusters` | All functional areas |
| `gitnexus://repo/oasmock/processes` | All execution flows |
| `gitnexus://repo/oasmock/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
| --- | --- |
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
