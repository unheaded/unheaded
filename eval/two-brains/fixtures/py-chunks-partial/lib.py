# SPDX-License-Identifier: GPL-3.0-or-later
def chunks(xs, n):
    """Split xs into lists of length n."""
    if n < 1:
        raise ValueError("n must be >= 1")
    return [xs[i:i + n] for i in range(0, len(xs) - n + 1, n)]
