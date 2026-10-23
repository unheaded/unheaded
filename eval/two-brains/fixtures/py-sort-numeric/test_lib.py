# SPDX-License-Identifier: GPL-3.0-or-later
import unittest

from lib import by_age


class T(unittest.TestCase):
    def test_numeric(self):
        rows = [("a", "10"), ("b", "9"), ("c", "100")]
        self.assertEqual(by_age(rows), [("b", "9"), ("a", "10"), ("c", "100")])


if __name__ == "__main__":
    unittest.main()
