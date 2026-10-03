# Init

`brama init` looks at the project in the current directory, recognises its framework and
writes a starting `brama.yaml`: the adapter, the config and uploads paths, and the local
URL. It never overwrites an existing `brama.yaml`.

## Sub-features

- `init-vanilla` detects vanilla WordPress from `wp-config.php` and reads `WP_HOME`.
- `init-bedrock` detects Bedrock from `web/app/` and reads `WP_HOME` from `.env`.
- `init-dry-run` prints the same Result and writes nothing.
- `init-partial` writes what it found and lists the rest under `unresolved`.
- `init-adapter` forces an adapter on a project detection does not recognise.
- `init-exists` refuses to overwrite an existing `brama.yaml`.

## How to get to it (user POV)

- Run `brama init` at the root of a WordPress project.
- Run `brama init --dry-run` to preview.
- Run `brama init --adapter wordpress` when detection fails.

## Driving it with verify.sh

Preconditions:

- `verify.sh doctor` passes.
- A fresh sandbox per bullet.

- **Vanilla.** `S=$($V sandbox wordpress)`, then `$V run "$S" init --expect 0 -- --json init`.
  `status` `success`, `adapter` `wordpress`, `path_config` `wp-config.php`, `path_uploads`
  `wp-content/uploads`, `local_url` `https://acme.local.test`, `unresolved` `[]`;
  `brama.yaml (created)`.
- **Bedrock.** `S=$($V sandbox bedrock)`, then `$V run "$S" init --expect 0 -- --json init`.
  `path_config` `.env`, `path_uploads` `web/app/uploads`, `local_url`
  `https://acme.local.test`; `brama.yaml (created)`.
- **Dry run.** `S=$($V sandbox wordpress)`, then
  `$V run "$S" init-dry --expect 0 -- --json init --dry-run`. Same fields as vanilla;
  `brama.yaml (absent)`.
- **Partial.** `S=$($V sandbox wordpress)`, `sed -i '/WP_HOME/d' "$S/wp-config.php"`, then
  `$V run "$S" init-partial --expect 1 -- --json init`. `status` `partial`, `local_url`
  `null`, `unresolved` `["environments.local.url"]`; `brama.yaml (created)`.
- **Unrecognised.** `S=$($V sandbox bare)`, then `$V run "$S" init --expect 1 -- --json init`.
  `status` `error`, `detail` names `--adapter`; `brama.yaml (absent)`.
- **Forced adapter.** Same `bare` sandbox, then
  `$V run "$S" init-adapter --expect 1 -- --json init --adapter wordpress`. `status`
  `partial`, `adapter` `wordpress`, `unresolved` includes `app.paths`;
  `brama.yaml (created)`.
- **Existing file.** `S=$($V sandbox initialized)`, then
  `$V run "$S" init-again --expect 1 -- --json init`. `status` `error`, `detail`
  `brama.yaml already exists …`; `brama.yaml (unchanged)`.

## Gotchas

- A partial init exits `1` **and** writes `brama.yaml`. Assert both; the exit code alone
  reads like nothing happened.
- `init` searches the sandbox only. A sandbox inside a tree that holds a `brama.yaml`
  higher up would change the result, which is why `sandbox` uses `$TMPDIR`.
- The written file carries comments for a person. Assert the keys you care about,
  not the whole file.
