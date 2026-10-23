# SPDX-License-Identifier: GPL-3.0-or-later
def word_count(text):
    """Count words, case-insensitively."""
    counts = {}
    for w in text.split():
        counts[w.lower()] += 1
    return counts
