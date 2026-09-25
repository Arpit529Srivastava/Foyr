# Foyr

Hosting platform: deploy existing apps with built-in auth, invites and roles.

Read before making architectural changes:
- docs/architecture/system-design.md — how the system works
- docs/architecture/decisions.md — ADRs; add a new ADR instead of silently changing a decision

Conventions:
- Never hardcode domains: `PLATFORM_DOMAIN` = foyr.online (site, app., api., auth.),
  `APPS_DOMAIN` = foyrapps.online (customer apps). They are different domains on purpose (ADR-015).
- AWS region is ap-south-1.
- Git: this repo commits as the personal account (Arpit529Srivastava) and pushes over SSH via
  the `github-personal` host alias. Do not use the global (work) git identity.
