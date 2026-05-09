# Releasing boo

This document covers how to cut a release, set up the Homebrew tap for the first
time, and what secrets need to exist in the repo before anything can publish.

---

## Prerequisites

Install GoReleaser locally for sanity checks:

```sh
brew install goreleaser
```

---

## One-time tap setup

GoReleaser pushes the Homebrew formula to a separate tap repository.
You must create that repo **before** the first tag.

1. Create an empty repo at `https://github.com/erzz/homebrew-tap`
   (public, no README, no licence — GoReleaser manages the contents).
2. Create a Personal Access Token (PAT):
   - Go to GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens
   - Scope: **Contents → Read and write** on the `erzz/homebrew-tap` repo only
   - Classic token alternative: `repo` scope is sufficient
3. Add the PAT as an Actions secret on **this** repo (`erzz/boo`):
   - Settings → Secrets and variables → Actions → New repository secret
   - Name: `HOMEBREW_TAP_GITHUB_TOKEN`
   - Value: the PAT from step 2

That's it. GoReleaser will create and update `Formula/boo.rb` automatically on
every tagged release.

---

## Required Actions secrets

| Secret name                  | Purpose                                         |
| ---------------------------- | ----------------------------------------------- |
| `HOMEBREW_TAP_GITHUB_TOKEN`  | PAT with `repo` scope on `erzz/homebrew-tap`    |
| `GITHUB_TOKEN`               | Auto-provided by Actions — no setup needed       |

---

## Cutting a release

1. **Ensure `main` is clean and all tests pass:**

   ```sh
   make test
   make lint
   ```

2. **Create and push an annotated tag:**

   ```sh
   git tag -a v1.2.3 -m "release v1.2.3"
   git push origin v1.2.3
   ```

   That's all. The `release.yml` workflow triggers on the tag push and
   GoReleaser handles the rest:
   - Builds `boo` for `darwin/arm64` and `darwin/amd64`
   - Merges them into a universal binary
   - Creates a GitHub Release with the archive and `checksums.txt`
   - Pushes an updated formula to `erzz/homebrew-tap`

3. **Verify the release:**
   - Check the Actions run at `https://github.com/erzz/boo/actions`
   - Check the release at `https://github.com/erzz/boo/releases`
   - Check the formula at `https://github.com/erzz/homebrew-tap`

---

## Local dry-run (snapshot)

Test the full GoReleaser pipeline locally without creating a tag or publishing
anything:

```sh
make snapshot
# or: goreleaser release --snapshot --clean
```

Artefacts land in `dist/`. Inspect them; nothing is pushed.

## Validate the config

```sh
make release-check
# or: goreleaser check
```

---

## Version scheme

Use [Semantic Versioning](https://semver.org): `vMAJOR.MINOR.PATCH`.

- `PATCH` — bug fixes, no new features
- `MINOR` — new features, backwards-compatible
- `MAJOR` — breaking changes

Pre-releases are not formally supported yet; use snapshot builds for testing.

---

## Changelog

GoReleaser builds the changelog automatically from commit messages between the
previous tag and `HEAD`. Commits matching the following prefixes are excluded
from release notes (they are noise, not user-visible changes):

- `docs:`, `chore:`, `test:`, `ci:`, `build:`, `style:`, `refactor:`

Use [Conventional Commits](https://www.conventionalcommits.org/) so that `feat:`
and `fix:` commits surface correctly in the release notes.
