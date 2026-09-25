# Foyr: Product Vision

> Deploy the app you already wrote, with login, invites and roles built in.

## What Foyr is

A hosting platform for people who already wrote their own app (with Cursor, Claude Code,
by hand, whatever) and just want to deploy it with auth, permissions and a custom domain,
without touching the AWS console or wiring up a separate auth provider.

**The wedge:** zero-config auth + permissions bundled into deploy, for code you already
wrote. Every app gets a front door: private by default, invite people by email, gate
features by role, without writing a single line of auth code.

## Target user

Solo devs and small teams who've built something (often AI-assisted) and need to share it
with a few teammates or clients (an internal tool, a small SaaS MVP, a client dashboard)
without spinning up Cognito/Auth0 + IAM + a permissions system themselves.

## What we are not building

- **Not an AI app generator.** That is Modelence's lane (YC-backed, 15k+ users).
- **Not a generic PaaS** competing purely on infra flexibility. That is
  Railway / Render / Fly.io / Vercel's lane.

## Non-goals for v1

- No AI app generation
- No multi-cloud or bring-your-own-cloud (AWS only)
- No per-tenant VPC isolation (shared VPC, app-level isolation)
- No custom roles beyond owner / admin / member / viewer
- No non-containerizable workloads

## Competitive landscape

| Who | What they do | Why we're different |
|---|---|---|
| Modelence | AI app generation + hosting | We host code you already have |
| Railway / Render / Vercel / Fly.io | Generic PaaS | None bundle auth + per-app access control |
| Cognito / Auth0 / Clerk | Standalone auth providers | Require manual integration; with Foyr you never touch auth directly |

## Success looks like

A stranger can sign up, connect a repo, deploy, and invite a teammate who logs in and
sees the app, without help and without writing auth code.
