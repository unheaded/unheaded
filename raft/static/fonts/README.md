# Fonts: none are loaded

The Zhen pages (`raft/pages/chat.html`, `raft/pages/source-view.html`) use
system fonts. No font file is in this directory or anywhere in the repository,
and no page loads one from another host.

Until 2026-11-21 both pages loaded JetBrains Mono and Space Grotesk from
`fonts.googleapis.com` and `fonts.gstatic.com`, and this page described how to
copy the files here. That was settled the other way (decision pack 2026-11-18
row 41, M-206.1): copying them in would add third-party files, and a page that
shows model output and retrieved text should name no other host in its
Content-Security-Policy. The stylesheets keep the two names at the front of
their font stacks (`--font-mono`, `--font-heading` in `static/chat.css` and
`static/source-view.css`), so a viewer who has them installed sees them;
everyone else sees `Courier New` or the system's sans-serif, as on the
dashboard.

## If the typefaces are wanted back

This is Stevie's call: font files are third-party material and need a licence
row in `THIRD_PARTY.md` first (both families are under the SIL Open Font
License 1.1). Then:

1. Put the `.woff2` files here and a `@font-face` rule for each in
   `static/chat.css` and `static/source-view.css`, with `url(/static/fonts/...)`.
2. Add `font-src 'self'` to `_PAGE_CSP` in `raft/zhen_app.py`. Never a host.
3. Update `raft/tests/test_zhen_page_policy.py`, which today fails on any font
   file, any `@font-face` and any `font-src` directive.
