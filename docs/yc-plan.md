# YC application plan

**Deadline:** Nov 2, 2026, 8pm PT (Nov 3, 8:30am IST). Decision by Dec 11.
**Target:** submit on Nov 1, a day early.

## What the application must show
1. **A live demo:** `git push` → live URL → the app sits behind magic-link login → invite a teammate by email.
2. **Real users:** 3 to 5 people from the waitlist running real apps on Foyr.
3. **Speed:** a solo founder who built infra, a build pipeline and auth in weeks (git history, ADRs).

## Week by week
| Dates | Goal | Work | Done |
|---|---|---|---|
| Oct 10 to 16 | `git push` → live URL (Phase 0) | Step 7: run the image on Fargate with a health check, deployment `live`. Step 8: authproxy routes `<app>.apps.foyr.online` (ADR-028). Step 4b: foyrd on ECS with ALB and RDS, paid from AWS credits, so it runs without the laptop. | [x] Oct 19 |
| Oct 16 | Founder-only page | After 8c: a page on foyrd behind a secret link showing deployments, steps, timings and failure reasons, for watching deploys in a browser and for application screenshots. Not the real dashboard. | [x] Oct 19 |
| Oct 17 to 23 | Login in front of every app | Magic-link login (Resend), sessions, authproxy session check and `X-Foyr-*` headers, invite by email | [ ] |
| Oct 24 to 30 | Users and demo | Minimal dashboard (apps, deployments with steps), onboard 3 to 5 waitlist users, buy the separate apps domain before the first outside user (ADR-028), record the demo video | [ ] |
| Oct 31 to Nov 1 | Apply | Answers, 1-minute founder video, demo link, submit | [ ] |

## Cut until after applying
Roles and `hasPermission`, custom domains, scale-to-zero, the Python SDK, Node and Python auto-detection,
polished dashboard pages, audit log UI and export. The data for the audit log is already recorded (ADR-026).

## Numbers to have ready
- Waitlist sign-ups (Resend audience)
- Apps deployed by outside users, deployments run, median push-to-live time
- First live build: push to image in 1 min 22 s (deployment 2, 2026-10-10)
- First live app: push to healthy running app in 2 min 10 s (deployment 4, 2026-10-10)
- Laptop off: push to live in 1 min 52 s with foyrd, its database and authproxy all in AWS (deployment 5, 2026-10-19, ADR-032)
- First public URL: https://foyr-hello.apps.foyr.online over HTTPS through the load balancer and authproxy (2026-10-18, ADR-031); about 80 ms from India

## Rules while racing
- Same quality bar: tests, PRs, ADRs (ADR-018). Explanations get shorter, not the tests.
- AWS changes still need explicit approval before `tofu apply`.
- Cost: AWS credits cover AWS; the only cash item is the apps domain, needed before the first outside user.
