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

## Wotan carries the notifications (Stevie, 2026-11-11)

> don't we have a connection to wotan, can't we use that - wasn't that sort of wotans purpose?

Yes, between services. That is what Wotan is for, and the dashboard and Kanban backends already hold a Wotan connection (`WOTAN_GRPC_ADDR`, gRPC stream with HTTP fallback, `pkg/wotan-client`). The shape:

1. **Producers publish** on Wotan: Kanban (`notify.*` when changes are requested or a card waits for review), a Champion runbook, Akira. Topics carry ids, kinds and links, never message text (question 4). A notification that is also an alert or comes from a Champion uses a signed prefix (the producer signs or refuses).
2. **The service that owns notifications subscribes**, writes each one to the Well (history and read marks), and keeps per-user unread counts. The Well holds them because Wotan is in memory by default and keeps no history (ADR-005; the Postgres store was deleted 2026-11-09).
3. **The browser asks its own page's backend**, as above, by poll or later server-sent events. It never connects to Wotan.

Why the browser stays off Wotan: Wotan is internal. It accepts only service tokens (`topics.auto_approve` in `configs/wotan.yaml` lists the service identities), and today it does not limit which topics an accepted service may read: FND-043, found 2026-11-11 while writing this, to be fixed on develop. A browser on it would mean user sessions on the bus, and users able to subscribe to infrastructure topics. CLAUDE.md "Breaking Architectural Isolation" names that pitfall. So Wotan replaces any service-to-service calls for notifications; the last hop to the browser stays poll or server-sent events through the BFF. The poll is only that last hop.

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

A notification service adds three links: producers and it to Wotan (same TLS 1.3 hybrid as above), it to the Well (same `hostssl`), and the browser to it through its page's BFF (same TLS as the pages). Rule for this and later ADRs: every new link states whether it is encrypted and how, in a table like this one.

## The last hop is a live relay, not a poll (Stevie's answer, 2026-11-12)

Asked whether the badge should poll, Stevie chose the other one: **notifications ride Wotan, and the last hop to the browser is a live server-sent-events relay of the page server's Wotan subscription.** The poll recommended above is set aside; that section stays as the record of what was weighed.

The shape, with nothing else changed:

1. Producers publish `notify.*` on Wotan and the notification service writes each one to the Well, as above.
2. **Each page's backend holds one Wotan subscription** for notifications (dashboard-backend and kanban-app already hold a stream; the Zhen page's backend gets one, or relays through the service that owns notifications). Under FND-043's `topics.permissions` each backend's identity is listed for `notify.*` and nothing wider.
3. **The backend relays to its own browser tabs over server-sent events** (`EventSource`): one way, plain HTTP semantics, same origin, the same session cookie as every other read. The browser still never connects to Wotan.
4. **The relay filters per session.** A tab receives only the events addressed to its logged-in user or that user's teams. The Wotan topic carries ids and kinds, never text (question 4), and the backend decides who may see which id; an event for someone else is dropped at the backend, not hidden in the page.
5. **The Well is still the truth.** A tab that connects, or reconnects after sleep, first reads its unread count from the Well through the backend, then follows the live stream; an event missed while disconnected is found by that read, not by replay from Wotan (in memory, no history).

What the answer adds to settle before a plan:

- **A connection per tab on each backend.** A cap per session and per server, a heartbeat comment so idle streams are not cut by a proxy, and a clean close at logout and at session expiry so a tab cannot keep listening on a dead session.
- **The Zhen front** (`cmd/zhen-ui-front`, a proxy over a Unix socket) must pass a streaming response without buffering; today it serves ordinary requests.
- **Cross-site use of the stream.** `EventSource` sends the session cookie, so the endpoint checks `Origin` and `Sec-Fetch-Site` like the WebSocket endpoints do, and its Content-Security-Policy needs `connect-src 'self'` only.
- **Encryption of the links:** unchanged from the table above. The stream is the browser-to-page link (TLS 1.3, hybrid ML-KEM); the backend's subscription is a services-to-Wotan link (the same).

His other sentence that day, that Wotan "may still be improved and developed on to increase its speed and efficiency too so it is a more comparable alternative to redis", is a pitch of its own: ADR-142.

## Decision

None. Parked under ADR-112; it needs ADR-139's users, teams and email first. The transport of the last hop is decided (server-sent events, 2026-11-12); everything else stands as written.
