# Releasing boo

Releases are automated via semantic-release + GoReleaser, but **cutting a
release is a manual action** — `release.yml` only runs when you trigger it
from the Actions tab. Once triggered, the rest is automatic.

---

## How releases happen

1. Land your work on `main` using [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, etc.). Nothing ships yet.
2. When you're ready to release, go to **Actions → Release → Run workflow** (on `main`).
3. `release.yml` runs semantic-release, which analyses commits since the last tag, decides the version bump, creates the tag, and publishes a GitHub Release with auto-generated notes.
4. The new tag triggers `goreleaser.yml`, which builds the universal binary, attaches it to the release semantic-release created, and pushes the Homebrew formula to `erzz/homebrew-tap`.

> **Why semantic-release needs a PAT (not `GITHUB_TOKEN`).** Tags pushed using
> the default `GITHUB_TOKEN` do **not** trigger downstream workflows — this is
> a GitHub safeguard against recursive CI. If semantic-release used
> `GITHUB_TOKEN`, the tag would be created but `goreleaser.yml` would never
> fire and the Homebrew formula would never update. `release.yml` therefore
> passes `RELEASE_TOKEN` (a PAT) so the tag push looks like it came from a
> user and fires `goreleaser.yml` as intended.

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

This fires `goreleaser.yml` directly and ships v0.1.0. From there, run the
`Release` workflow manually whenever you want to ship accumulated commits.

---

## One-time setup (before first release)

1. Create an empty public repo at `https://github.com/erzz/homebrew-tap`.
2. Create a fine-grained Personal Access Token with access to **both** `erzz/boo` and `erzz/homebrew-tap`, with these repository permissions:
   - `erzz/boo`: Contents R/W, Issues R/W, Pull requests R/W, Metadata R
   - `erzz/homebrew-tap`: Contents R/W, Metadata R
3. Add the PAT as a single Actions secret on `erzz/boo` named `RELEASE_TOKEN`. Both workflows read from it:
   - `release.yml` passes it as `GITHUB_TOKEN` so semantic-release can push the version tag in a way that fires `goreleaser.yml` (see the note above on why `GITHUB_TOKEN` is not enough).
   - `goreleaser.yml` passes it as `HOMEBREW_TAP_GITHUB_TOKEN` so GoReleaser can commit the updated formula to `erzz/homebrew-tap`.
4. Cut the `v0.1.0` tag manually (see above).

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
| semantic-release fails to push tag | Check `contents: write` permission on `release.yml`, and that `RELEASE_TOKEN` is set and not expired |
| Tag created but `goreleaser.yml` never ran | `RELEASE_TOKEN` is missing or `release.yml` is using `GITHUB_TOKEN` — tags pushed with `GITHUB_TOKEN` don't trigger workflows. Re-push the tag from your laptop (`git push --delete origin vX.Y.Z && git tag -d vX.Y.Z && git tag vX.Y.Z <sha> && git push origin vX.Y.Z`) to fire GoReleaser once, then fix the token. |
| Tag created but GoReleaser failed | Re-run `goreleaser.yml` on the tag — safe because `release.mode: keep-existing` |
| Need to ship right now | Actions → Release → Run workflow |
