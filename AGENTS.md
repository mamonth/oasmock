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
