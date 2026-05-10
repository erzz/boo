# Releasing boo

Releases are automated via semantic-release + GoReleaser. Merge to `main` and
the pipelines handle the rest.

---

## How releases happen

1. Merge one or more commits to `main` using [Conventional Commits](https://www.conventionalcommits.org/).
2. `release.yml` runs semantic-release, which analyses commits since the last tag, decides the version bump, creates the tag, and publishes a GitHub Release with auto-generated notes.
3. The new tag triggers `goreleaser.yml`, which builds the universal binary, attaches it to the release semantic-release created, and pushes the Homebrew formula to `erzz/homebrew-tap`.

---

## Commit types that trigger releases

| Type | Release |
|---|---|
| `feat:` | minor |
| `fix:` | patch |
| `refactor:`, `perf:` | patch |
| `docs:`, `chore:`, `test:`, `ci:`, `style:`, `build:` | none |
| `feat!:` or `BREAKING CHANGE:` in body | major |

---

## First release (v0.1.0)

semantic-release uses `1.0.0` as the hardcoded first release when no tags exist.
To start at `v0.1.0`, cut it manually once off `main`:

```sh
git tag v0.1.0
git push origin v0.1.0
```

This fires `goreleaser.yml` directly and ships v0.1.0. From the next `feat:` or
`fix:` commit on `main`, semantic-release takes over and bumps from there
(`v0.1.1`, `v0.2.0`, …).

---

## One-time setup (before first release)

1. Create an empty public repo at `https://github.com/erzz/homebrew-tap`.
2. Create a Personal Access Token with `repo` scope on `erzz/homebrew-tap`.
3. Add it as an Actions secret named `HOMEBREW_TAP_GITHUB_TOKEN` on `erzz/boo`.
4. Cut the `v0.1.0` tag manually (see above).

`GITHUB_TOKEN` is auto-provided by Actions — no setup needed.

---

## Local dry-runs

```sh
# GoReleaser snapshot (no publishing, no tag required)
make snapshot

# semantic-release dry-run (requires Node; will fail without GH auth but parses config)
npx --yes semantic-release --dry-run --no-ci
```

---

## Failure modes

| Symptom | Fix |
|---|---|
| semantic-release fails to push tag | Check `contents: write` permission on `release.yml` |
| Tag created but GoReleaser failed | Re-run `goreleaser.yml` on the tag — safe because `release.mode: keep-existing` |
| Need to skip a release | Add `[skip ci]` to the commit message |
