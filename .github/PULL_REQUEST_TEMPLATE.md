## Summary

<!-- What does this PR change and why? -->

## Checklist

- [ ] `ANANSI_ENV=development go test ./...` passes
- [ ] (packages/anansi) `bun test` and `bunx tsc --noEmit` pass
- [ ] Golden vectors: if wire output changed, regenerated via
      `GOLDEN_UPDATE=1 go test ./core/encoding/anansi/ -run TestGenerateGoldenVectors`
      **and** the TS conformance suite still passes

## Contributor License Agreement

By opening this pull request, I confirm that my contribution is made under
the terms of the [CLA](../../CLA.md) — granting the maintainer the right to
use, distribute, and relicence my contribution under AGPLv3 or commercial
licenses.

- [ ] I agree to the CLA
