# SPDX-License-Identifier: GPL-3.0-or-later
import unittest

from lib import word_count


class T(unittest.TestCase):
    def test_counts(self):
        self.assertEqual(word_count("a b A c b a"), {"a": 3, "b": 2, "c": 1})

    def test_empty(self):
        self.assertEqual(word_count("  "), {})


if __name__ == "__main__":
    unittest.main()
