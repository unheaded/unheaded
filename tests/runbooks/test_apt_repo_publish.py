# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/deploy/apt-repo-publish.yaml. Its east-side steps
are run against stand-ins by scripts/test-deb-maintscripts.sh; this holds what
the runner itself would refuse."""
import harness

RB = "runbooks/deploy/apt-repo-publish.yaml"


def test_runner_finds_no_unset_variable_once_the_parameters_are_given():
    # A dpkg-query format such as ${Package} reads to the runner as an unset
    # parameter, and it exits 1 before the first step.
    rb = harness.load(RB)
    given = {p["name"]: "given" for p in rb["parameters"]}
    assert harness._RUNNER.find_unset_params(rb, given) == {}
