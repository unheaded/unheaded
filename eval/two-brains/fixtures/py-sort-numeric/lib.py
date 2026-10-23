# SPDX-License-Identifier: GPL-3.0-or-later
def by_age(rows):
    """rows: list of (name, age_string). Youngest first."""
    return sorted(rows, key=lambda r: r[1])
