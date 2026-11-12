<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-143: A Terraform template that stands the whole kingdom up on AWS, Azure, GCP, LXD or Docker

**Status:** Proposed (parked). A pitch; an empty skeleton repository only, no resources, no plan.
**Date:** 2026-11-12 (pitch of 2026-11-11)
**Related:** ADR-089 (develop, staging, main), ADR-096 and the native-first rule of 2026-10-29 (nothing third-party without Stevie's confirmation), ADR-113 (third-party verdicts), ADR-047 (repository breakout), ADR-112 (plan freeze), `pkg/iac` (IaC output renderers), the signed apt repository (FND-026), the sealed cask (`scripts/build-sealed-cask.sh`), `docs/battle-plans/S-E1-LIVE-battle-plan.md`.

## The pitch (Stevie, 2026-11-11, as taken down)

`~/tmp/projects/unheaded-terraform`: a Terraform template for AWS, Azure, GCP, LXD and Docker that deploys the full stack by pulling it from GitHub. Details later. End goal: a live multi-cloud deployment of `main` once everything is promoted and tagged 1.0.0.

His answer on 2026-11-12: a parked ADR plus an empty repository skeleton (README and layout, no resources).

## As understood

One repository, outside this one, that a person points at a cloud account or a host and gets a running kingdom from: the same stack on five targets, built from a released tag, never from a working tree. It is the proof of "your tools, our data model" at the infrastructure layer, and the first thing an adopter would try. Nothing is built until 1.0.0 exists to deploy.

## What is there

- **A renderer, not a deployment.** `pkg/iac` renders the control plane's desired state as Terraform HCL (and Ansible, Puppet, Kubernetes, Chef, Salt). That is output for an adopter's own pipeline. It provisions nothing and is not what this ADR describes.
- **Two real deployments, both by hand and by plan:** west and east, from debs and a compose stack, through `S-E1-LIVE`. LXD is the primary runtime (Cuirass drives it); Docker compose runs the stack on west today.
- **Things a template could pull, when they exist:** the signed apt repository (FND-026, not yet moved), sealed casks with an ML-DSA-65 Binding Rune (`cask-verify`), release tags. `main` is old; 1.0.0 is not tagged.
- **The skeleton:** `~/tmp/projects/unheaded-terraform`, created 2026-11-12: a README, the licence, a layout of empty directories for the five targets and a `.gitignore` that keeps state and variable files out. No `.tf` file declares a resource.

## To settle in this ADR before any plan

1. **The tool is third-party, and so is every provider.** Terraform itself, and the AWS, Azure, GCP, LXD and Docker providers it downloads at `init`. Each is an item for the third-party sitting (ADR-113): pinned versions, a committed lock file with hashes, no provider fetched unpinned. Terraform has been under the Business Source License since 2023; OpenTofu is the MPL-2.0 fork with the same language. Which one the template is written for and tested with is Stevie's choice and belongs in the Barrister's notes, though running either does not touch this repository's GPL.
2. **What "pulling it from GitHub" pulls.** A release tag's signed artifacts (debs from the apt repository, or sealed casks), verified before anything starts: the template must fail on a bad signature, not deploy. Never `main`'s head, never an unverified archive.
3. **One shape or five.** The same thing on each target (a VM or instance that installs the debs and runs the units) keeps five targets honest and comparable; using each cloud's managed services (a managed PostgreSQL for the Well, a load balancer in front) is five different products to prove and to secure. Recommended to start: one shape, VMs, with LXD and Docker as the two local targets that already exist.
4. **Secrets and state.** Terraform state holds every secret the template handles. Where state lives (never in the repository), how it is encrypted, and how the kingdom's own secrets are made on the host instead of passing through state at all (the rotation of 2026-11-12 made them on the host: `unheaded-keygen`, the compose secrets file). No credential, no account id and no state file in the repository; the gitleaks gate and the shrink-only baseline apply there as here.
5. **The network.** Default deny, nothing published but the gateway, the WireGuard overlay between nodes, TLS 1.3 hybrid as on west. A cloud's default security group must not widen what west's firewall would refuse. IPv6 as a first-class target.
6. **Cost and teardown.** A multi-cloud deployment bills by the hour. A stated budget, the smallest instance sizes that hold the stack (the footprint work of 2026-11-12 feeds this), and `destroy` proven to leave nothing behind before anything is left running.
7. **How it is tested.** `validate` and `plan` can run without an account; a real apply needs one and costs money. What runs in CI, where, and with whose credentials (none stored in GitHub that could spend).
8. **Relationship to `pkg/iac`.** Whether the template's modules are what the Terraform renderer emits, or a separate hand-written thing. One source is better than two that drift.
9. **Where it lives.** Its own repository (it is not a Go package and has its own release rhythm), GPL-3.0-or-later like the rest, on GitHub when Stevie pushes it.

## Decision

None. Parked under ADR-112; opens when 1.0.0 is tagged.
