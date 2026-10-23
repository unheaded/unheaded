# SPDX-License-Identifier: GPL-3.0-or-later
import unittest

from lib import slug


class T(unittest.TestCase):
    def test_examples(self):
        self.assertEqual(slug("Hello, World!  2026"), "hello-world-2026")
        self.assertEqual(slug("  The Well -- PG 18 "), "the-well-pg-18")
        self.assertEqual(slug("ok"), "ok")


if __name__ == "__main__":
    unittest.main()
