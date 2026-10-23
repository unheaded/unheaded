# SPDX-License-Identifier: GPL-3.0-or-later
import unittest

from lib import chunks


class T(unittest.TestCase):
    def test_partial(self):
        self.assertEqual(chunks([1, 2, 3, 4, 5], 2), [[1, 2], [3, 4], [5]])

    def test_exact(self):
        self.assertEqual(chunks([1, 2, 3, 4], 2), [[1, 2], [3, 4]])

    def test_empty(self):
        self.assertEqual(chunks([], 3), [])

    def test_bad_n(self):
        with self.assertRaises(ValueError):
            chunks([1], 0)


if __name__ == "__main__":
    unittest.main()
