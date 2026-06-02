# Hearth Governance

Hearth is an early-stage, open-source project under **Apache-2.0**. This document describes how the
project is run today. It is intentionally lightweight and will evolve (toward a more formal,
CNCF-style model) as the community grows.

## Principles

- **Open** — development, discussion, and decisions happen in the open (issues, PRs, discussions).
- **Vendor-neutral** — no single vendor controls the project; the multi-backend abstraction is a
  first-class, neutral concern.
- **Scoped** — Hearth is the Kubernetes orchestration/lifecycle layer; it does not re-implement
  inference engines or write chip kernels (see [CONTRIBUTING.md](CONTRIBUTING.md)).

## Roles

- **Contributors** — anyone who opens an issue or PR, improves docs, or helps others. No formal
  status required.
- **Maintainers** — listed in [MAINTAINERS.md](MAINTAINERS.md). They review and merge changes, triage
  issues, manage releases, and uphold the [Code of Conduct](CODE_OF_CONDUCT.md).

## Decision-making

Decisions are made by **lazy consensus**: proposals (issues/PRs) move forward if no maintainer
objects within a reasonable time. Substantive or contentious changes should be raised as an issue
first to allow discussion. Where consensus can't be reached, maintainers decide; while there is a
single maintainer, that maintainer is the tie-breaker.

Code changes require approval from at least one maintainer (other than the author, once there is more
than one) and passing CI.

## Becoming a maintainer

There is no fixed bar. Sustained, high-quality contribution — code, reviews, docs, triage, and good
community judgment — is the path. An existing maintainer nominates a candidate; current maintainers
approve by consensus. New maintainers are added to [MAINTAINERS.md](MAINTAINERS.md).

Maintainers who become inactive may be moved to emeritus status by agreement of the other
maintainers.

## Changing this document

Changes to governance are proposed via PR and approved by the maintainers.
