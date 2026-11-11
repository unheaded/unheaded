<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-140: Notifications and messages across the web pages

**Status:** Proposed (parked, far future). Nothing is built and nothing is planned.
**Date:** 2026-11-11
**Related:** ADR-139 (admin page: users, teams, mail settings; its question 3), ADR-114 (one login for every page), ADR-112 (plan freeze), `pkg/bff/static/auth.js` (the login badge), `cmd/alert-mailer` (mail).

## The pitch (Stevie, 2026-11-11, verbatim)

> email and in app notification would be nice - that opens another adr for a messaging/notification app that hooks into our web gui's - maybe something like logout button ( it polls api to check for messages/notifications ) and blinks/pulses ever so slightly to instruct them to click - when clicked it navigates to another webgui with whatever info they need and message/group/team message history or somethinbg

## What is there

- **The badge.** `pkg/bff/static/auth.js` draws the login badge (user name and log out) at the bottom right of the dashboard and Kanban; the Zhen page has its own (`raft/static/index.html`). It already calls the page's `/auth/me` and works under each page's Content-Security-Policy (styles set through the DOM, no inline style text).
- **Live updates.** Kanban already keeps a server-sent events stream open (`cmd/kanban-app/static/js/websocket.js`, `EventSource`); the dashboard has its own live views.
- **Mail.** The alert mailer sends alert mail with hourly and daily caps (`observability/alerting/policy.yaml`, dry-run on west today). There is no mail to a user about their own work, and no user has an email address yet (ADR-139).
- **Events.** Wotan carries signed topics (`audit.*` and others); a card change is already published (`kanban.tasks.updated`).

## The idea

1. **A notification mark on the badge.** The badge asks one endpoint for the count of unread notifications for the logged-in user, and when it is not zero shows the count and pulses very slightly. Clicking it opens the messages page.
2. **A messages page.** One web page, reached from every app, with the user's notifications (what happened, where, a link to it) and message history: direct, team, and group threads.
3. **Mail as well.** Each notification can also be mailed, per the user's and the admin's settings on ADR-139's page.
4. **First uses.** Kanban: Request changes reaches the card's owner or owning team (ADR-139); a card waiting in Review reaches whoever may approve it. Later: Zhen finishing a run, an alert addressed to a team.

## Questions for when it is taken up

1. **Polling or a stream.** Stevie's sketch polls. A poll every 30-60 s is simple and cheap at this scale; a server-sent events stream (Kanban already has one) shows a notice at once. Both behind the same endpoint.
2. **Where it lives.** Its own small service with its own Well tables (notifications, threads, messages, read marks) and account, or part of `unheaded-auth`, which already knows users and teams.
3. **The pulse.** Subtle, stops once the page has been looked at, and off under `prefers-reduced-motion` (the dashboard already honours it). It needs a `@keyframes` rule in a served stylesheet: Kanban's policy is `style-src 'self'`.
4. **Retention and privacy.** How long messages are kept, who can read a team thread, and that no message text goes to logs or Wotan (only ids and counts), like the audit rules of ADR-114.
5. **Mail limits.** Account mail shares the mailer's account and caps, or has its own (DP-710 watches outbound mail closely).
6. **Messages between people**, or notifications only at first?

## Addendum (Stevie, 2026-11-11): badge look, links, Champion, mail

> the badge should have a hover over text when lit/pulsing, highlighting the outer box leaving the inner "Log out" rectangle flat and unlit ---- links is nice, can ping pong around between task and stuff, say an automated champion task notices something weird it could notify admins and link the related dashboards (would need runbook) just pitching ideas. mail notification would be configurable from admin page, we can piggy back off alert email setup for now while in development first uses look good.

- **Badge look:** when there are unread notifications, the badge's outer box is what lights and pulses; the inner "Log out" button stays flat and unlit, so the two never look like one control. Hovering the lit badge shows a short text (for example "2 unread: click to read"). The click target for messages is the box, not the button.
- **Links both ways:** a notification links to what it is about (a card, a dashboard, a run), and those pages link back to the thread, so a reader can move between a task and its discussion.
- **Champion as a sender (idea):** an automated Champion task that notices something odd notifies the admins, with links to the related dashboards. It acts only through a runbook (ADR-024: approved, in the sealed manifest), so what it may notice and say is reviewed like any other runbook step.
- **Mail:** configured on ADR-139's admin page. Until that exists, during development, notification mail uses the alert mailer's setup (its account, recipients, dry-run switch and caps in `observability/alerting/policy.yaml`).
- **First uses:** accepted as listed in "The idea" item 4.
- Question 1 (poll or stream), 2 (where it lives), 4 (retention) and 6 (messages between people) stay open; 3 is answered for the look, the reduced-motion rule stands; 5 is answered for now (the mailer's caps).

## Question 1: polling or a kept-open connection (Developer pass, 2026-11-11)

Stevie asked for the Developer's view: "web socket in con wait vrs api call every N seconds".

**Recommendation: poll.** The badge asks one endpoint, `GET /notifications/unread` on its own page's origin, every 30 s with a few seconds of random jitter, and only while the tab is visible (`visibilitychange`); after an error it backs off to 2, then 5 minutes. The answer is a count and a version tag (`ETag`), so an unchanged count is a `304` with no body.

Why, at this scale (a handful of users, three pages):

- **Same path as every other read.** A poll is an ordinary request through the BFF session, auth middleware, rate limits, metrics and the Content-Security-Policy (`default-src 'self'` already allows a same-origin `fetch`). Nothing new to secure.
- **Nothing long-lived to hold.** No per-tab connection on the server, no reconnect logic, nothing for the Zhen front (`cmd/zhen-ui-front`, a proxy over a Unix socket) to keep flushing.
- **A WebSocket adds risk for no gain here:** the upgrade needs its own origin check (cross-site WebSocket hijacking rides the session cookie), its own auth on upgrade, and the front's proxy must pass upgrades. The badge only needs server-to-browser, one way.
- **Cost:** three tabs at one request per 30 s is 6 requests a minute, each a `304` of a few hundred bytes.
- **Delay:** a notice can wait up to about 30 s to light the badge. Acceptable for "changes requested" or "a card waits for you"; not for an outage, which already has alert mail.

If instant delivery is wanted later, the step up is **server-sent events** (`EventSource`, one-way, plain HTTP, same session cookie; Kanban already uses one), not a WebSocket, behind the same endpoint so the badge does not change. Stevie decides; this is the recommendation only.

## Encryption of each link on the notification path (read on west 2026-11-11)

Stevie: "we need to be noting if app to app communications are encrypted or not and how it is being accomplished if so". Checked with `openssl s_client` and the containers' settings, not taken from docs:

| Link | Encrypted | How |
|---|---|---|
| Browser to dashboard (20000), Kanban (20001), Zhen (20103) | Yes | TLS 1.3; the handshake negotiated `X25519MLKEM768` (hybrid post-quantum) on all three. Zhen through `cmd/zhen-ui-front`, `pkg/tls` Strict (ADR-138). |
| Zhen front to the Zhen app | Not network traffic | Unix socket `/run/unheaded-zhen-ui/app.sock`, opened only by the front's account. |
| Dashboard and Kanban to `unheaded-auth` (19020) | Yes | `AUTH_ISSUER_URL=https://auth:19020` with `UNHEADED_TLS_ENABLED=true` (Strict profile, deployment CA). Port not published on the host, so not probed directly. |
| Services to Wotan (18000 HTTP, 18001 gRPC) | Yes | TLS 1.3, `X25519MLKEM768` on both; plain HTTP to 18000 is refused (400). |
| Services to the Well (5432) | Yes | TLS 1.3; `pg_hba.conf` allows only `hostssl` from the compose network, plain `host` is rejected. Key exchange still classical (ADR-105 item E1). Local socket inside the container is `trust` (no network). |
| Alert mailer to the mail relay (`smtp.gmail.com:587`) | Yes when sending | `STARTTLS`, TLS 1.2 minimum, and it refuses to send a password without it. West is in dry-run: nothing is sent. |
| Browser to the wiki (20002) | **No** | Plain HTTP on the LAN address; the temporary `unheaded-wiki-test` unit. Read-only, no login, but unencrypted. Its permanent home (next.md item 8 (a)) should serve TLS like the other pages. |

A notification service adds two links: browser to it (same TLS as the pages above, through the BFF) and it to the Well (same `hostssl`). Rule for this and later ADRs: every new link states whether it is encrypted and how, in a table like this one.

## Decision

None. Parked under ADR-112; it needs ADR-139's users, teams and email first.
