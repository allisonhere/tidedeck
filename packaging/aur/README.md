# AUR packaging

TideDeck ships to the AUR as **`tidedeck-bin`**, which repackages the release tarballs
published by `.github/workflows/release.yml`: the exact binaries that were
tested and released, installed in seconds without a Go toolchain.

## Files

| File | Purpose |
| --- | --- |
| `PKGBUILD.in` | The template. Edit this, never a rendered `PKGBUILD`. |
| `render-pkgbuild.sh` | Substitutes version, `pkgrel`, and checksums into the template. |

Placeholders: `@PKGVER@`, `@PKGREL@`, `@SHA256_X86_64@`, `@SHA256_AARCH64@`,
`@SHA256_LICENSE@`. Rendering fails if any placeholder survives, so a typo can
never publish a PKGBUILD that cannot build.

Plugins are not part of the package: they are installed from GitHub on the
Plugins page, which is why `git` is an optional dependency.

## Publishing

Use `./deploy.sh` → **AUR → Publish tidedeck-bin**. It confirms the GitHub release has
both Linux tarballs and `SHA256SUMS`, takes the digests from that published file
(never a local build), hashes `LICENSE` at the tag, picks `pkgrel`, renders the
PKGBUILD and `.SRCINFO`, shows the diff, and pushes only after you confirm.

Run `./deploy.sh --dry-run` first to rehearse without pushing anything, and
`./deploy.sh --check` to see what a release still needs.
