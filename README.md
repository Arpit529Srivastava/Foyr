# Foyr

**Deploy the app you already wrote — with login, invites and roles built in.**

Foyr hosts apps you built yourself (with Cursor, Claude Code or by hand) and puts a front
door on each one: private by default, invite people by email, gate features by role — with
zero auth code and no AWS console.

> Status: pre-Phase 0. Design approved; website and docs in progress.

## Documentation

| Doc | What's in it |
|---|---|
| [Product vision](docs/product/vision.md) | What we're building, for whom, and what we're not |
| [System design](docs/architecture/system-design.md) | How the platform works end to end |
| [Decisions (ADRs)](docs/architecture/decisions.md) | Key technical choices and why |
| [Roadmap](docs/roadmap.md) | Phases and exit criteria |

## Stack at a glance

Go (foyrd, authproxy) · Next.js (dashboard, website) · AWS ap-south-1: ECS Fargate, ALB,
CodeBuild, ECR, RDS/Aurora Postgres, SES, Route53, ACM · OpenTofu · Website on Vercel
