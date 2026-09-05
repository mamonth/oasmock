#!/usr/bin/env python3
"""
Enforce the unit-test comment convention from docs/project.md: every test
function (func Test* / func Benchmark*) in the unit-test tree (internal/ and
cmd/) MUST be preceded by a Gherkin scenario header (a /**/ block containing a
"Scenario:" line).

Exit code 1 when any unit test function lacks a header. Integration tests
(test/) and generated mocks (mock/, *_mock.go) are outside the unit-test tree
and skipped. Files under mock/ and generated files are also skipped.
"""

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

TEST_FN = re.compile(r"^func\s+(Test\w+|Benchmark\w+)\s*\(")


def test_files():
    for base in (ROOT / "internal", ROOT / "cmd"):
        for p in base.rglob("*_test.go"):
            yield p


def missing_headers() -> list:
    missing = []
    for path in sorted(test_files()):
        if "_mock" in path.name or "mock" in str(path).split("/internal")[0]:
            pass
        if "mock" in path.parts:
            continue
        content = path.read_text()
        lines = content.splitlines()
        for i, line in enumerate(lines):
            m = TEST_FN.match(line.strip())
            if not m:
                continue
            # Scan backward from the line above the function up to the
            # previous blank-line-separated comment block, looking for the
            # Gherkin marker. 24 lines covers long race-regression headers.
            start = max(0, i - 24)
            header = "\n".join(lines[start:i])
            if "Scenario:" not in header:
                missing.append(
                    f"{path.relative_to(ROOT)}:{i + 1}: {m.group(1)} lacks a "
                    f"Scenario: header (docs/project.md testing standards)"
                )
    return missing


def main() -> int:
    missing = missing_headers()
    for entry in missing:
        print(entry, file=sys.stderr)
    if missing:
        print(
            f"\n{len(missing)} unit test function(s) miss a Gherkin header.",
            file=sys.stderr,
        )
        return 1
    print("All unit test functions carry a Gherkin Scenario: header.", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
