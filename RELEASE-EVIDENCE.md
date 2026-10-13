# Release evidence

Verification date: 2026-10-13

## Published contract

- Repository: `null-select/cli`, public, Apache-2.0.
- Published platforms: Linux AMD64 and Linux ARM64.
- Release workflow: tag-triggered tests, race detection, vet, vulnerability
  scanning, reproducible cross-compilation, SHA-256 checksums, GitHub artifact
  attestations, and immutable release publication.
- Release assets never include unsigned macOS or Windows binaries.
- Release tags matching `v*` reject deletion and non-fast-forward updates.
- The release environment accepts only matching tags. The default workflow
  token is read-only; the release job requests only contents, OIDC, and
  attestation write authority.

## v0.1.0 evidence

Source commit: `c6fa58533ece10a5ddc5f1370253c6cd6a807fa9`

This release was published before repository-level immutable releases were
enabled. Its tag is protected against deletion and movement, and its downloaded
assets are covered by the checksum and provenance verification below. The
repository setting and draft-then-publish workflow make subsequent releases
immutable together with their complete asset set.

| Asset | SHA-256 |
|---|---|
| `nullselect_linux_amd64` | `6d49105c2d237b72fadd730d023edc4ff7047ed4ad96066752745cf3fe92429e` |
| `nullselect_linux_arm64` | `95624517498e393e3ad29bd2277d21282905af4b1a583bc33a06caa830e11188` |

Both downloaded assets matched `checksums.txt`. `gh attestation verify` resolved
each artifact to a SLSA provenance v1 statement signed from GitHub Actions and
recorded in the public transparency log. Both ELF binaries reported version
`0.1.0`, the exact source commit, and the source commit timestamp when executed
under their Linux architecture contract.

The downloaded AMD64 and ARM64 binaries each connected to the production
gateway with a separately issued expiring credential, negotiated MCP
`2026-07-28`, identified `null-select-managed` version `0.1.0`, and listed one
tool. The credential was revoked, and the next ARM64 doctor connection was
rejected as unauthorized.

The ARM64 release artifact also passed:

- Codex CLI 0.162.1 native add, exact configuration inspection, and remove;
- Claude Code 2.1.296 native add, live authenticated connection, exact
  environment-variable header inspection, and remove;
- Cursor, VS Code, GitHub Copilot, and portable project-file install/remove;
- explicit rejection of a fabricated Claude Desktop Linux path.

Cursor, VS Code, and GitHub Copilot remain configuration targets rather than
supported application rows until their applications complete the live release
matrix. macOS and Windows remain verification build targets and are not public
release assets.

## v0.1.1 immutable release

Source commit: `f6f4789100d11a63fb0c0b950b4a6048dbcd1fd1`

| Asset | SHA-256 |
|---|---|
| `nullselect_linux_amd64` | `ebe27bd93a0d73cb949b3965d2fc9185b11e88efc18bcb1f03a60d26abf3f1f3` |
| `nullselect_linux_arm64` | `1904628a1f20bd78e4923b7d6662134abcf4f4bf2694c8c83596a757a8b906e2` |

GitHub reports the release as immutable. Both downloaded artifacts matched the
published checksum file, both SLSA provenance attestations verified, and both
Linux architecture executions reported version `0.1.1` with the exact source
commit and commit timestamp.

Each architecture completed a production `mcp doctor` connection with
credential `cred_34fd7c78c493df0222751b65`, negotiated MCP `2026-07-28`,
identified `null-select-managed` version `0.1.0`, and listed one tool. The
credential was revoked immediately after the checks, and the next AMD64 doctor
connection failed as unauthorized.
