---
name: foyr-infra-change
description: Use for any OpenTofu change in infra/. Write, test, plan, get explicit approval, apply, verify.
---

# Infra change

1. `aws sso login --profile foyr` if the session expired; use `AWS_PROFILE=foyr`.
2. Edit `.tf` files in `infra/` (or `infra/bootstrap/` for the state bucket only). One-line comments.
3. Add or update a `*.tftest.hcl` with `mock_provider "aws"` asserting the security-relevant settings.
4. `tofu fmt -recursive infra`, then per folder `tofu init -backend=false && tofu validate && tofu test`.
5. `tofu plan -out=tfplan` with the real backend. Show the user the resource list and the `Plan:` line, and the cost.
6. Stop. Apply only after the user explicitly says so. Then `tofu apply tfplan` and delete `tfplan`.
7. Verify with read-only AWS CLI calls, then re-run `tofu plan` and expect "No changes".
8. Ship with `foyr-ship-change`. Never commit state, plans or `.terraform/`.

State: bucket `foyr-tfstate-899628690655`, keys `bootstrap/` and `prod/`, S3-native locking.
