#!/usr/bin/env python3
"""
Analyze requirement scenario coverage by tests.

This script:
1. Extracts all requirement scenarios from spec files (baseline and active change specs)
2. Maps test files to the scenarios they cover
3. Calculates coverage percentage
4. Optionally generates a detailed traceability matrix
"""

import os
import re
import sys
import argparse
from pathlib import Path
from typing import Dict, List, Set, Tuple
import textwrap


class ScenarioCoverageAnalyzer:
    def __init__(self, root_dir: str):
        self.root_dir = Path(root_dir)
        self.spec_dir = self.root_dir / "openspec" / "specs"
        self.changes_dir = self.root_dir / "openspec" / "changes"

        # Data structures
        self.all_scenarios: Set[str] = set()
        self.scenario_source: Dict[str, str] = {}
        self.scenario_to_tests: Dict[str, Dict[str, List[str]]] = {}
        self.test_to_scenarios: Dict[str, Set[str]] = {}

    def extract_scenarios_from_specs(self) -> None:
        """Extract all RS.* scenario codes from spec files.

        Baseline specs (openspec/specs) take priority over active change
        specs (openspec/changes/*/specs); archived specs are excluded.
        """
        self._extract_from_dir(self.spec_dir, "spec")
        self._extract_from_dir(self.changes_dir, "change")

    def _extract_from_dir(self, search_dir: Path, source: str) -> None:
        """Extract RS.* scenario codes from spec.md files under a directory."""
        pattern = re.compile(r"#### Scenario\s+(RS\.[A-Z]+\.[0-9]+)")

        for spec_file in search_dir.rglob("spec.md"):
            # Exclude archived specs
            if "archive" in str(spec_file):
                continue

            content = spec_file.read_text()
            matches = pattern.findall(content)

            for scenario in matches:
                if scenario not in self.all_scenarios:
                    self.all_scenarios.add(scenario)
                    self.scenario_source[scenario] = source
                    self.scenario_to_tests[scenario] = {"unit": [], "integration": []}

    def classify_test_file(self, test_file: Path) -> str:
        """Classify a test file as unit or integration based on its path."""
        test_file_str = str(test_file)

        # Integration tests are in test/ directory
        if "test/" in test_file_str:
            return "integration"

        # Unit tests are in internal/ or cmd/ directories
        if "internal/" in test_file_str or "cmd/" in test_file_str:
            return "unit"

        # Default to unit
        return "unit"

    def extract_scenarios_from_test_file(self, test_file: Path) -> Set[str]:
        """Extract RS.* scenario codes from a test file."""
        content = test_file.read_text()

        # Look for "Related spec scenarios:" lines
        pattern = re.compile(r"Related spec scenarios:\s*(.+)")
        scenarios = set()

        for line in content.split("\n"):
            match = pattern.search(line)
            if match:
                # Parse comma-separated scenario codes
                scenario_text = match.group(1).strip()
                # Split by commas, handle spaces
                for scenario in re.split(r",\s*", scenario_text):
                    if scenario.startswith("RS."):
                        scenarios.add(scenario)

        return scenarios

    def analyze_test_files(self) -> None:
        """Find all test files and map them to scenarios."""
        # Find all Go test files
        test_files = list(self.root_dir.rglob("*_test.go"))

        for test_file in test_files:
            # Skip files that don't contain actual test functions
            content = test_file.read_text()
            if "func Test" not in content and "func Benchmark" not in content:
                continue

            test_type = self.classify_test_file(test_file)
            test_rel_path = test_file.relative_to(self.root_dir)

            # Extract scenarios covered by this test
            scenarios = self.extract_scenarios_from_test_file(test_file)

            # Store test -> scenarios mapping
            if scenarios:
                self.test_to_scenarios[str(test_rel_path)] = scenarios

                # Store scenario -> test mapping
                for scenario in scenarios:
                    if scenario in self.scenario_to_tests:
                        self.scenario_to_tests[scenario][test_type].append(
                            str(test_rel_path)
                        )
                    else:
                        # Scenario mentioned in test but not found in any spec
                        print(
                            f"Warning: Scenario {scenario} in {test_rel_path} not found in baseline or active change specs"
                        )
                        self.scenario_to_tests[scenario] = {
                            "unit": [],
                            "integration": [],
                        }
                        self.scenario_to_tests[scenario][test_type].append(
                            str(test_rel_path)
                        )

    def calculate_coverage(self) -> Tuple[int, int, float]:
        """Calculate coverage statistics."""
        covered_scenarios = set()

        for scenario, tests in self.scenario_to_tests.items():
            if tests["unit"] or tests["integration"]:
                covered_scenarios.add(scenario)

        total = len(self.all_scenarios)
        covered = len(covered_scenarios)
        coverage_pct = (covered / total * 100) if total > 0 else 0.0

        return total, covered, coverage_pct

    def generate_report(self, detailed: bool = False) -> str:
        """Generate coverage report."""
        total, covered, coverage_pct = self.calculate_coverage()

        report = []
        report.append("# Requirement Scenario Coverage Report")
        report.append("")
        report.append(f"**Total scenarios:** {total}")
        report.append(f"**Covered scenarios:** {covered}")
        report.append(f"**Coverage:** {coverage_pct:.1f}%")
        report.append("")

        if detailed:
            report.append("## Detailed Traceability Matrix")
            report.append("")
            report.append("| Scenario | Source | Unit Tests | Integration Tests |")
            report.append("|----------|--------|------------|-------------------|")

            # Sort scenarios naturally ascending (by area, then numeric)
            def scenario_sort_key(scenario: str) -> Tuple[str, int]:
                match = re.match(r"(RS\.[A-Z]+)\.(\d+)", scenario)
                if match:
                    return match.group(1), int(match.group(2))
                return scenario, 0

            sorted_scenarios = sorted(
                self.scenario_to_tests.keys(), key=scenario_sort_key
            )

            for scenario in sorted_scenarios:
                tests = self.scenario_to_tests[scenario]

                # Format test lists
                unit_tests = tests["unit"]
                integration_tests = tests["integration"]

                unit_str = "<br>".join(sorted(unit_tests)) if unit_tests else "—"
                integration_str = (
                    "<br>".join(sorted(integration_tests)) if integration_tests else "—"
                )
                source = self.scenario_source.get(scenario, "unknown")

                report.append(
                    f"| {scenario} | {source} | {unit_str} | {integration_str} |"
                )

        return "\n".join(report)

    def run_analysis(self) -> None:
        """Run complete analysis."""
        print("Extracting scenarios from spec files...", file=sys.stderr)
        self.extract_scenarios_from_specs()

        print(f"Found {len(self.all_scenarios)} requirement scenarios", file=sys.stderr)

        print("Analyzing test files...", file=sys.stderr)
        self.analyze_test_files()

        print(f"Analyzed {len(self.test_to_scenarios)} test files", file=sys.stderr)


def main():
    parser = argparse.ArgumentParser(
        description="Analyze requirement scenario coverage by tests"
    )
    parser.add_argument(
        "--detailed",
        "-d",
        action="store_true",
        help="Generate detailed traceability matrix",
    )
    parser.add_argument(
        "--coverage-only",
        "-c",
        action="store_true",
        help="Output only coverage percentage as decimal (e.g., 0.639)",
    )
    parser.add_argument(
        "--output", "-o", type=str, help="Output file (default: stdout)"
    )
    parser.add_argument(
        "--root",
        type=str,
        default=".",
        help="Project root directory (default: current directory)",
    )

    args = parser.parse_args()

    analyzer = ScenarioCoverageAnalyzer(args.root)
    analyzer.run_analysis()

    if args.coverage_only:
        total, covered, coverage_pct = analyzer.calculate_coverage()
        coverage_decimal = coverage_pct / 100.0
        output = f"{coverage_decimal:.3f}"
    else:
        report = analyzer.generate_report(detailed=args.detailed)
        output = report

    if args.output:
        with open(args.output, "w") as f:
            f.write(output)
        if not args.coverage_only:
            print(f"Report written to {args.output}", file=sys.stderr)
    else:
        print(output)


if __name__ == "__main__":
    main()
