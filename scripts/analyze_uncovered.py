#!/usr/bin/env python3
"""
Analyze uncovered requirement scenarios.
"""

import re
from pathlib import Path

def extract_uncovered_scenarios(report_file: str):
    """Extract scenarios that have no tests from coverage report."""
    uncovered = []
    
    with open(report_file, 'r') as f:
        lines = f.readlines()
    
    # Look for table rows with both unit and integration tests as "—"
    pattern = re.compile(r'^\| (RS\.[A-Z]+\.[0-9]+) \| — \| — \|$')
    
    for line in lines:
        match = pattern.match(line.strip())
        if match:
            uncovered.append(match.group(1))
    
    return uncovered

def group_scenarios_by_domain(scenarios):
    """Group scenarios by domain (CLI, MSC, MAPI, EXT, TR)."""
    grouped = {}
    
    for scenario in scenarios:
        # Extract domain: RS.CLI.1 -> CLI
        domain = scenario.split('.')[1]
        if domain not in grouped:
            grouped[domain] = []
        grouped[domain].append(scenario)
    
    return grouped

def main():
    report_file = "coverage_report.md"
    uncovered = extract_uncovered_scenarios(report_file)
    
    print(f"Total uncovered scenarios: {len(uncovered)}")
    print("\nUncovered scenarios by domain:")
    
    grouped = group_scenarios_by_domain(uncovered)
    for domain in sorted(grouped.keys()):
        scenarios = sorted(grouped[domain], key=lambda x: int(x.split('.')[-1]))
        print(f"\n{domain} ({len(scenarios)} scenarios):")
        for scenario in scenarios:
            print(f"  - {scenario}")

if __name__ == "__main__":
    main()