#!/usr/bin/env python3
import os
import re

def load_specs():
    spec_dir = "openspec/specs"
    specs_by_domain = {}
    for domain in os.listdir(spec_dir):
        domain_path = os.path.join(spec_dir, domain)
        if not os.path.isdir(domain_path):
            continue
        spec_file = os.path.join(domain_path, "spec.md")
        if not os.path.isfile(spec_file):
            continue
        with open(spec_file, 'r') as f:
            content = f.read()
        domain_specs = []
        for m in re.finditer(r'#### Scenario (RS\.\w+\.\d+): (.*)', content):
            domain_specs.append({
                'code': m.group(1),
                'title': m.group(2).strip(),
            })
        specs_by_domain[domain] = domain_specs
    return specs_by_domain

def print_specs_by_domain(specs_by_domain):
    for domain, specs in specs_by_domain.items():
        print(f"\n{domain.upper()}:")
        for spec in specs:
            print(f"  {spec['code']}: {spec['title']}")

def main():
    specs_by_domain = load_specs()
    print("Available spec scenarios by domain:")
    print_specs_by_domain(specs_by_domain)

if __name__ == '__main__':
    main()