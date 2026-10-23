<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# rb-unquoted-colon

The runbook does not parse: a command contains ": " and is not quoted,
so YAML reads it as a mapping. Quote it so the runbook parses and
passes the shape check, and the command text stays the same. Do not
change validate_runbook.py.
