# SPDX-License-Identifier: GPL-3.0-or-later
def slug(title):
    """URL slug for a title."""
    return title.lower().replace(" ", "-")
