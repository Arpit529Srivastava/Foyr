---
name: foyr-ship-change
description: Use when finishing any change in the Foyr repo. Branch, run checks, signed commit as the personal account, push over SSH, open a PR (main is protected).
---

# Ship a change

`main` is protected (ADR-017). Never push to it.

1. `git checkout main && git pull`, then `git checkout -b <type>/<short-name>` (feat, fix, docs, ci, chore).
2. Make the change. Keep comments to one short line.
3. Add or update tests with the change (Go: `_test.go`, bug fix: failing test first). Go: `gofmt -l cmd internal`, `go vet`, `go test -race ./cmd/... ./internal/...`. Infra: `tofu fmt -check`, `tofu validate`. If `site/` changed: `cd site && npm run check` (lint, format, typecheck, build). Fix with `npx prettier --write` if format fails.
4. Commit small and signed: `git commit -s -m "<type>(<scope>): <summary>"`. Use the personal identity, not the work one.
5. `git push -u origin HEAD` (SSH host alias `github-personal`).
6. `gh pr create --repo Arpit529Srivastava/Foyr` with a short body.
7. Tell the user the PR link. They merge it; Vercel then redeploys the site.

Do not use em dashes in any text.
