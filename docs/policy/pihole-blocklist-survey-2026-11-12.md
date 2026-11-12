<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# Pi-hole blocklists: what is subscribed and what could be added (2026-11-12)

Stevie, 2026-11-12: "let's look around for some other list - think ublock origin has a good list we can add in too". Every list is a third-party feed that decides what the household can resolve, so each one is added only on his word (native-first rule, 2026-10-29). This is the comparison to decide from. Nothing was added.

## Today

Read from the Pi-hole container's gravity database on 2026-11-12 (Pi-hole core v6.5, FTL v6.8):

| Subscribed | Entries | Licence | Last pulled |
|---|---:|---|---|
| StevenBlack unified hosts (`raw.githubusercontent.com/StevenBlack/hosts/master/hosts`) | 78,216 | MIT | 2026-11-08 |

One list. On top of it: 8 exact denies and 5 regex denies made by hand, no allows. The upstream README describes the base list as adware and malware only.

This Pi-hole can take more than hosts files: its gravity database has an `abp_entries` column and an `antigravity` table, so it reads Adblock-style `||domain^` lines and can subscribe to allow-lists as well as block-lists.

## uBlock Origin's lists are for a browser

uBlock Origin's own lists (`uBlockOrigin/uAssets`, GPL-3.0) are written in its extended filter syntax for use inside a browser: most rules name a URL path, a page element or a script to rewrite. A DNS server sees only a name, so only the lines of the form `||domain^` could mean anything to Pi-hole, and the repository's README says nothing about use outside the browser. Subscribing Pi-hole to them would import a small usable part of a list that was never tested for DNS-wide blocking, where one wrong name breaks a whole site instead of one element.

What gives uBlock Origin most of its reach by default is not its own list but EasyList and EasyPrivacy. The DNS-level form of those exists: AdGuard's DNS filter is built from EasyList, EasyPrivacy and AdGuard's own lists and, in its README's words, "simplified specifically to be better compatible with DNS-level ad blocking". That is the way to get what he liked about uBlock Origin onto the Pi-hole.

## Candidates

Facts are from each project's own page, read 2026-11-12. "Not stated" means the page does not say.

| List | Entries | Licence | Updated | Built for DNS | False positives, in its own words |
|---|---:|---|---|---|---|
| HaGeZi Multi Light | 88,455 | GPL-3.0 | rebuilt several times a day, published daily | yes | not stated for this tier |
| HaGeZi Multi Normal | 208,182 | GPL-3.0 | same | yes | not stated for this tier |
| HaGeZi Multi Pro | 223,960 | GPL-3.0 | same | yes | not stated for this tier |
| HaGeZi Multi Pro++ / Ultimate | 241,487 / 264,295 | GPL-3.0 | same | yes | "might block a few legit domains by mistake"; have an admin ready to unblock |
| HaGeZi Threat Intelligence Feeds (Medium / Full) | 373,421 / 1,482,194 | GPL-3.0 | same | yes | not stated; a separate add-on for malware and phishing names |
| AdGuard DNS filter | not stated | GPL-3.0 | not stated | yes, by design | not stated |
| OISD big / small | not stated | **not stated** | not stated (timestamps only) | yes | "You shouldn't find any false positives! In fact, I dare you to report any" |
| uBlock Origin uAssets | n/a | GPL-3.0 | continuous | **no** (browser syntax) | reports are for page breakage in the browser |

Each HaGeZi tier contains the one below it; the README lists Pi-hole under its Adblock-format files. HaGeZi's counts "change with every build".

## Recommendation

1. **Add one list first: HaGeZi Multi Normal**, in its Adblock format. It is built for DNS, has a stated licence, a public issue tracker for wrong blocks, and is about 2.7 times the size of what is subscribed now without being one of the tiers its own author warns about. Pro is the author's recommended tier and only 8% larger; it is the next step once Normal has run a week without a complaint.
2. **For the uBlock Origin wish: the AdGuard DNS filter**, as a second addition after the first has settled, not at the same time, so that anything that breaks can be traced to one list.
3. **Consider HaGeZi's Threat Intelligence Feeds (Medium) separately.** It is a security list (malware, phishing), not an ad list, and large; it answers a different question from "fewer ads".
4. **Not OISD for now:** no licence or terms could be found on its pages, and it is a merge of other lists whose reports do not go back to the sources.
5. **Not uAssets:** wrong tool for a DNS server.

Keep StevenBlack: it is small, MIT, and the others overlap rather than replace it.

## How to add one safely

- One list at a time, a week apart. Before adding: note the gravity count and the block rate. After: `pihole -g`, then the same two numbers.
- Test the household's everyday sites and apps the same evening (banking, streaming, phone app stores, smart TV sign-in). A site that breaks gets an exact allow in Pi-hole, written down with the date and the list that caused it.
- If the allows for one list pass a handful in a week, drop back a tier instead of growing the allow-list.
- Each list is fetched by the Pi-hole container from GitHub or a CDN on a timer: one more outside dependency of the household's DNS. Gravity keeps the last good copy when a fetch fails.
- A list that goes on is a row for the third-party inventory (`docs/policy/third-party-inventory-*.md`, section "Outside services the code calls") with its verdict.

## Found while looking

- Pi-hole's configured upstream resolvers include four IPv6 addresses (Cloudflare and Google). The container has no IPv6 address, so those four can never answer. Harmless, and it goes with the Pi-hole IPv6 todo.

## Sources read

- <https://github.com/StevenBlack/hosts>
- <https://github.com/hagezi/dns-blocklists>
- <https://github.com/AdguardTeam/AdGuardSDNSFilter>
- <https://oisd.nl/>, <https://oisd.nl/setup/pihole>, <https://oisd.nl/includedlists>
- <https://github.com/uBlockOrigin/uAssets>
- <https://docs.pi-hole.net/main/pihole-command/> (the gravity page of the Pi-hole documentation did not load; what this Pi-hole's gravity accepts was read from its own database schema instead)
