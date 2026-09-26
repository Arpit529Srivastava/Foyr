# Foyr

Hosting platform: deploy existing apps with built-in auth, invites and roles.

Read before making architectural changes:
- docs/architecture/system-design.md: how the system works
- docs/architecture/decisions.md: ADRs. Add a new ADR instead of silently changing a decision

Conventions:
- Never hardcode domains: `PLATFORM_DOMAIN` = foyr.online (site, app., api., auth.),
  `APPS_DOMAIN` = foyrapps.online (customer apps). They are different domains on purpose (ADR-015).
- AWS region is ap-south-1.
- Git: this repo commits as the personal account (Arpit529Srivastava) and pushes over SSH via
  the `github-personal` host alias. Do not use the global (work) git identity.
- `main` is protected (ADR-017): never push to it. Work on a branch, commit with `-s`, open a PR.
  CI for `site/` must pass (`cd site && npm run check` runs the same steps locally).
- Go: `gofmt -l . && go vet ./cmd/... ./internal/... && go test ./cmd/... ./internal/...` runs the same steps as Go CI.

Lifecycle (ADR-018): every change follows plan → branch → tests first or alongside → implement → local checks → signed commit → PR → CI green → merge → live check (`foyr-live-check`). No feature code without tests: Go packages get `_test.go` files (table-driven where useful), site changes must keep `npm run check` green, infra changes must pass `tofu fmt -check` and `tofu validate`. Bug fixes start with a failing test. Nothing runs against AWS (`tofu apply`) without explicit approval.
