#!/usr/bin/env python3
"""
session-to-runbook.py — Extract operational steps from a session log into a YAML runbook draft.

Parses bash commands, their outputs, and verification steps from a session
transcript and generates a structured runbook YAML.

Usage:
  python3 scripts/session-to-runbook.py --input session.log --name my-runbook --output runbooks/infra/my-runbook.yaml
  echo "apt update && apt install nginx" | python3 scripts/session-to-runbook.py --name quick-install

ADR: ADR-024 Phase 3 — Session-to-runbook extraction
"""

import argparse
import re
import sys
import yaml
from datetime import datetime


def extract_commands(text):
    """Extract shell commands from session text."""
    commands = []
    lines = text.split('\n')

    for i, line in enumerate(lines):
        line = line.strip()

        # Match common command patterns
        # $ command, > command, # command, or lines starting with common tools
        if re.match(r'^[\$#>]\s+', line):
            cmd = re.sub(r'^[\$#>]\s+', '', line)
            commands.append({'command': cmd, 'line': i + 1})
        elif re.match(r'^(sudo|apt|pip|git|docker|systemctl|curl|ssh|cd|ls|cat|grep|make|go |cargo |python)', line):
            commands.append({'command': line, 'line': i + 1})
        # Multi-line commands with backslash continuation
        elif line.endswith('\\') and commands:
            commands[-1]['command'] += ' ' + line.rstrip('\\').strip()

    return commands


def commands_to_runbook(commands, name, description=''):
    """Convert extracted commands to a YAML runbook structure."""
    steps = []
    for i, cmd in enumerate(commands):
        step = {
            'name': f'step-{i+1}',
            'action': 'shell',
            'command': cmd['command'],
        }

        # Auto-detect verification steps
        if any(kw in cmd['command'] for kw in ['test ', 'grep -q', '| grep', 'curl -sf', 'echo $?']):
            step['name'] = f'verify-{i+1}'
            step['description'] = 'Verification step'

        # Auto-detect sudo requirements
        if 'sudo ' in cmd['command']:
            step['name'] = f'sudo-{step["name"]}'

        steps.append(step)

    runbook = {
        'apiVersion': 'runbook/v1',
        'kind': 'Runbook',
        'metadata': {
            'name': name,
            'description': description or f'Auto-generated runbook from session ({len(steps)} steps)',
            'owner': 'zhenai',
            'estimated_duration': f'{max(1, len(steps) * 2)}m',
            'risk': 'medium',
            'requires_sudo': any('sudo' in s.get('command', '') for s in steps),
            'generated_at': datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ'),
        },
        'steps': steps,
    }

    return runbook


def main():
    parser = argparse.ArgumentParser(description='Extract runbook from session log')
    parser.add_argument('--input', '-i', help='Input session log file (default: stdin)')
    parser.add_argument('--name', '-n', required=True, help='Runbook name')
    parser.add_argument('--description', '-d', default='', help='Runbook description')
    parser.add_argument('--output', '-o', help='Output YAML file (default: stdout)')
    args = parser.parse_args()

    # Read input
    if args.input:
        with open(args.input) as f:
            text = f.read()
    else:
        text = sys.stdin.read()

    # Extract commands
    commands = extract_commands(text)
    if not commands:
        print('No commands found in input.', file=sys.stderr)
        sys.exit(1)

    print(f'Extracted {len(commands)} commands', file=sys.stderr)

    # Generate runbook
    runbook = commands_to_runbook(commands, args.name, args.description)

    # Output
    yaml_str = yaml.dump(runbook, default_flow_style=False, sort_keys=False)

    if args.output:
        with open(args.output, 'w') as f:
            f.write(yaml_str)
        print(f'Runbook written to {args.output}', file=sys.stderr)
    else:
        print(yaml_str)


if __name__ == '__main__':
    main()
