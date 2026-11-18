# Publishing the official plugins

The workflow `.github/workflows/publish.yml` runs for every tag `<plugin>/v<version>`: it tests,
packs and submits that plugin to the store for review (ADR-0074). Normally it uses trusted publishing
(GitHub's ID token, no secret). A trusted publisher can only be added for a plugin the store already
knows, so the **first** upload of each plugin uses an author token once.

## One-time setup

1. **Store up to date** – store.phoedox.com on `main` (migrations up to 00012), it must reach
   `token.actions.githubusercontent.com`.
2. **Author "Phoedox"** – sign in at store.phoedox.com → author area with the Phoedox account that
   owns the official plugins; verify the domain phoedox.com if offered.
3. **GitHub environment** – in `phoedox/phoedox-plugins`: Settings → Environments → New environment
   `store`:
   - Deployment branches and tags: *Selected branches and tags*, add the tag rule `*/v*`.
   - Required reviewers: the maintainer (every publish run waits for an approval).
4. **Author token for the first upload** – author area → "Token for the command line" → create
   (`phxa_…`). Save it as environment secret `PHOEDOX_STORE_TOKEN` of `store`.
5. **Author signature (optional)** – `go run github.com/phoedox/phoedox-sdk/go/cmd/phx-plugin keygen`;
   the public key goes into the author area, the private one into the environment secret
   `PHOEDOX_AUTHOR_KEY`.

## First release

1. `scripts/release.sh all --push` – tags every official plugin with its manifest version and pushes
   the tags **one by one** (GitHub starts no workflows when more than three tags arrive in one push).
   The SDK example `de.example.ups` is skipped.
2. Approve each run in GitHub (environment `store`), then review and approve each version in the
   store under **Store management**.
3. For every plugin: author area → "Publishing from CI" → trusted publisher with repository
   `phoedox/phoedox-plugins`, workflow `publish.yml`, environment `store`.
4. Delete the secret `PHOEDOX_STORE_TOKEN` and create a new author token in the author area (that
   replaces and so revokes the old one).

## Later releases

Raise the version in the plugin's manifest, then `scripts/release.sh <plugin> --push`. The run uses
trusted publishing; nothing else to set up. A plugin with a brand logo is sent with `-logo-rights`
(the brand check in `docs/branding/brands.md` comes first, ADR-0090).
