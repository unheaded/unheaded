# SPDX-License-Identifier: GPL-3.0-or-later
import unittest

from lib import add_tag


class T(unittest.TestCase):
    def test_independent_calls(self):
        self.assertEqual(add_tag("a"), ["a"])
        self.assertEqual(add_tag("b"), ["b"])

    def test_own_list(self):
        mine = ["x"]
        self.assertEqual(add_tag("y", mine), ["x", "y"])


if __name__ == "__main__":
    unittest.main()
