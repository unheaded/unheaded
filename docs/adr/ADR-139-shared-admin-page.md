<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-139: One administration page for users, roles, password resets and mail

**Status:** Proposed (parked, far future). Nothing is built and nothing is planned.
**Date:** 2026-11-11
**Related:** ADR-114 (user authentication for every web page; its 2026-11-03 decisions), ADR-103 (service auth, `unheaded-auth`), ADR-112 (plan freeze), ADR-116 (monitor the monitors), `observability/alerting/policy.yaml` (the alert mailer).

## The pitch (Stevie, 2026-11-11, verbatim)

> I think kanban, zhenai, and dashboard should maybe have a shared administrative page for creating users, rbac, password resets, email ( this can be tied into alerting configuration GUI for emails etc).... just a far future ADR nothing now on that auth page idea

## What is there

- **Users and roles.** `cmd/unheaded-auth` (19020) is the issuer. Users are created with its host CLI (`usercmd.go`). RBAC has the roles `viewer`, `operator`, `admin`. ADR-114 says the first admin is made on the host, never through a web page.
- **One login.** ADR-114's 2026-11-03 decisions: a central identity provider with one gate in front of every page, the gate inside `unheaded-auth`, TOTP for every user, idle 30 min / absolute 12 h. The Zhen Flask UI is to be retired in favour of the dashboard's Zhen chat. All parked.
- **Recovery.** ADR-114 item 7: a lost second factor is reset on the host only (`unheaded-auth user reset-mfa`), "No email, so no SMTP account to steal."
- **Mail.** The alert mailer (`cmd/alert-mailer`, policy in `observability/alerting/policy.yaml`) sends alert mail, capped per hour and per day, dry-run on west today. Its recipients and mode are set in that file, not in a page.
- **Pages.** Dashboard (20000), Kanban (20001) and Zhen (20103) each show a login badge from `pkg/bff` (Zhen its own). None has an administration view.

## The idea

One page, reached from all three applications, for an `admin`:

1. create and disable users, and give them roles;
2. reset a password or a second factor;
3. set mail: the sending account and the recipients, shared with the alert mailer's settings, so alert mail and account mail are configured in one place.

## Questions for when it is taken up

1. **Where it lives.** A page of `unheaded-auth` behind the gate (ADR-114's direction puts the gate there), or a dashboard view calling the issuer's API.
2. **Resets by mail.** ADR-114 chose host-only recovery so that there is no mail account to steal. Mail resets reverse that. Keep host-only for the second factor and allow mail for passwords only, or keep everything host-only and let the page do resets for other users only, by an admin who is logged in?
3. **First admin.** Stays host-only (ADR-114), so the page never runs without one.
4. **Mail settings in a page.** The alert policy is a reviewed file today. A page that writes it needs the Well (write needs a database, `feedback_db_required_writes`), an audit row per change, and a decision on whether the file or the database is the source.
5. **Audit.** Every change from the page goes to `ops.login_events` and a signed Wotan `audit.auth.*` event, as ADR-114 sets for logins.

## Addendum (Stevie, 2026-11-11): Request changes contacts the card's owner

> it should send it back back to inprogress and contact the owner... if it was a manually created card there should be a mapping of an email to the user ( we are approaching an admin/settings/managment type page for user creation, password resets, rbac, and other application related settings )

- **Done the same day:** the Kanban review button "Request changes" moves the card back to In Progress and appends `[Changes requested <date>]` to its description, keeping earlier text. Its message no longer claims anyone is notified (`cmd/kanban-app/static/js/app.js`).
- **Waits on this ADR:** contacting the owner. A card's `owner` is free text typed into the form, with no user account or email behind it. The page above gives each user an email; a card then records its owner as a user, and Request changes mails that user through the same mail settings.
- **Questions:** (1) a card created by Zhen (`zhen-agentd`) has no human owner: does Request changes send it to To Do, where the worker picks cards up, with the reviewer's note as its next instruction, instead of In Progress, where the worker never looks? (2) Who else hears: the reviewer, a watcher list? (3) Mail only, or also a notice inside the apps?
- **Facts for (1), read 2026-11-11:** the Backlog column is status `todo` in the Well (the page names it Backlog), so "To Do" above means Backlog. Zhen has made or worked no card yet: `zhen_agent_runs` 0 rows, `kanban_zhen_optin` 0 rows, no card with a Zhen owner or type, and the only `zhen-agentd` on west is the login test's, with no Well and no scheduler.

## Addendum 2 (Stevie, 2026-11-11): review before Done, and owners as teams

> admin/owner needs to review before anything is marked done

> maybe owner could be a team if we implmented rbac like that

- **Rule:** no card reaches Done without a review by an admin or by the card's owner. Done is reached only from Review, by Approve.
- **Owner as a team:** the RBAC behind this page gets teams (groups of users). A card's owner is a user or a team; any member of the owning team, or an admin, can approve it; Request changes reaches every member (mail per the settings above).
- **What is there today:** any logged-in writer can drag a card from any column to Done, or create it as Done; the review column's Approve is only one of those paths. Writes go through `PUT /api/v1/tasks/<id>` (`handleUpdateTaskByID`) and `handleUpdateTask` in `cmd/kanban-app/main.go`; the session carries the user's roles (`auth.IdentityFromContext`, as `zhenoptin.go`'s `humanOperator` reads them). Cards made from `references/timeline.md` by the timeline sync (`cmd/kanban-app/timeline.go`) are Done when their milestone is completed there; whether those count as reviewed (the timeline is Stevie's own record) is a question.
- **Could come first, without teams:** the server refuses a change to Done unless the card is in Review and the request is a human with the admin role. Owner and team approval join when this page gives cards real owners.
- **More questions:** (4) a team's members: managed on this page only, or also from the host CLI? (5) Can a card have several owning teams? (6) Do timeline-sync cards skip the review rule?

## Decision

None. Parked under ADR-112 until Stevie takes it up; it depends on ADR-114 being built first.
