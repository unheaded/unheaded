<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# py-slug

`slug(title)` must lower-case the title, keep only letters, digits and
single hyphens between words, and strip leading/trailing hyphens:
"Hello, World!  2026" -> "hello-world-2026". It keeps punctuation today.
