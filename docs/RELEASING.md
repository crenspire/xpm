# Releasing xpm

Releases are cut from `main` by pushing a `v*` tag. The tag triggers `.github/workflows/release.yml`; everything else is automated.

## Prerequisites

- CI is green on `develop` (`.github/workflows/ci.yml`, including the `env-smoke` job), and on `main` too once `main` exists (it does not before the first release; see step 1).
- You have push rights to `crenspire/xpm` (branch `main` and tags).
- The first release is `v0.1.0`. Before it exists, `curl -fsSL https://crenspire.github.io/xpm/install.sh | sh` falls back to building from source with `go install` (Go 1.22+), the Homebrew cask has nothing to serve, and `go install ...@latest` installs a pseudo-version of the default branch. Once the tag is published, the same curl command downloads the verified release archive; nothing else needs to change.

## Cutting a release

1. Bring `main` up to date with `develop`. The repository's default branch is `develop`; `main` is the release branch.

   First release only: `origin` has no `main` branch yet, so create it from `develop`:

   ```bash
   git checkout develop && git pull --ff-only origin develop
   git push origin develop:main
   ```

   Optionally protect `main` in the repository settings, and keep `develop` as the default branch.

   Later releases: fast-forward `main` to `develop` (or open a PR from `develop` to `main` and merge it):

   ```bash
   git fetch origin                       # make sure origin/develop is current
   git checkout main && git pull --ff-only origin main
   git merge --ff-only origin/develop
   git push origin main
   ```

2. Wait for CI on `main` to pass.
3. Tag and push. This is the one command that releases, once `main` is ready:

   ```bash
   git tag -a v0.1.0 -m v0.1.0 && git push origin v0.1.0
   ```

## What CI does

On a `v*` tag the release workflow:

1. Runs `go test -race ./...` (the `test` job); the release job only starts if it passes.
2. Runs GoReleaser (`.goreleaser.yaml`, pinned to v2.18.2 both locally by the Makefile and in CI) which:
   - builds six targets: linux, darwin and windows on amd64 and arm64, with the version stamped into the binary;
   - packs `tar.gz` archives (`zip` on Windows) holding the binary, `README.md`, `LICENSE`, shell completions and man pages;
   - writes `checksums.txt` (SHA-256 of every archive);
   - writes an SBOM per archive with syft;
   - creates the GitHub release with a changelog generated from the commits since the previous tag;
   - generates the Homebrew **cask** and pushes it to `crenspire/homebrew-tap`, but only when the `HOMEBREW_TAP_GITHUB_TOKEN` secret exists. Without the secret the cask is built and the upload is skipped.
3. Best effort: signs `checksums.txt` with keyless cosign and uploads `checksums.txt.sig` and `checksums.txt.pem` to the release. A failure in this step does not fail the release.

`go install github.com/crenspire/xpm/cmd/xpm@v0.1.0` works as soon as the tag is on GitHub and reports `0.1.0` from `xpm --version`.

The install script is served by GitHub Pages at `https://crenspire.github.io/xpm/install.sh`: the Pages workflow copies `scripts/install.sh` into the site whenever it changes on `develop`. It finds the newest release through the `releases/latest` redirect, so it picks up a new tag immediately with no redeploy. Smoke-test after releasing:

```bash
curl -fsSL https://crenspire.github.io/xpm/install.sh | sh -s -- --dir "$(mktemp -d)"
```

## Dry run

```bash
make release-check   # validate .goreleaser.yaml
make snapshot        # full local build into dist/, publishes nothing (SBOMs skipped: they need syft)
```

Both run GoReleaser through `go run` at the pinned version, so no separate install is needed.

## Enabling the Homebrew tap

The cask is published to a separate repository. Once, before (or after) a tag:

1. Create the public repository `crenspire/homebrew-tap`.
2. Create a fine-grained personal access token limited to that repository with **Contents: read and write**.
3. In `crenspire/xpm`, add it as the repository secret `HOMEBREW_TAP_GITHUB_TOKEN`.
4. The cask is published by the next tag pushed after the secret exists. Re-running the workflow for an already-released tag is not supported (GoReleaser cannot re-upload existing assets); to publish the cask for a release that is already out, cut a patch tag (for example v0.1.1).

Users then install with `brew install --cask crenspire/tap/xpm` (macOS). The release binaries are not notarized, so the cask strips the quarantine attribute when it installs.

## Verifying a release

Download `checksums.txt`, `checksums.txt.sig`, `checksums.txt.pem` and an archive from the release page, then:

```bash
cosign verify-blob \
  --certificate-identity-regexp 'https://github.com/crenspire/xpm/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --signature checksums.txt.sig --certificate checksums.txt.pem \
  checksums.txt
sha256sum --ignore-missing -c checksums.txt     # macOS: shasum -a 256 --ignore-missing -c checksums.txt
```

The signature covers `checksums.txt`, which covers every archive. If the signature files are missing, the best-effort signing step failed; the checksums are still published. `scripts/install.sh` checks the archive against `checksums.txt` but does not verify the cosign signature.

## Rolling back

If a release is broken, delete the GitHub release and the tag, then fix and re-tag (prefer a new patch version over reusing a published one, because the Go module proxy caches tags):

```bash
gh release delete v0.1.0 --cleanup-tag --yes   # removes the release and the remote tag
git tag -d v0.1.0                              # remove the local tag
```

If the cask was already pushed, revert its commit in `crenspire/homebrew-tap`.
