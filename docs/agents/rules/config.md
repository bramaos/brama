# Config rules

What brama decided about `brama.yaml`. The reason is where each rule points; `docs/adr/`
is `ADR-NNNN`.

- Keep one committed `brama.yaml`, Classification and Approvals included; add no second or local config file. (ADR-0005)
- Edit `brama.yaml` surgically: parse to validate and locate, then change only the lines that change; never re-marshal it or render it from the AST. (ADR-0005, `config.AddServer` doc)
- Write each edit as a function in `internal/config` that takes and returns the document, `func X(doc []byte, …) ([]byte, error)`, like `AddServer` and `Approve`.
- Reject unknown keys, and exit 1 for a broken file, not 42. (ADR-0005)
- Never overwrite an existing `brama.yaml`, and offer no `--force`. (ADR-0005)
- Write `brama.yaml` only from commands that exist to change it; a Pull never does. (ADR-0011)
- Put Classification under `anonymize.tables` and Approval under `environments.<name>.anonymize.approved`; a Preset never approves. (ADR-0010 separate axes)
- Name an Approval `table.column` or `table.column=key`. (ADR-0015)
- Keep Policy, database credentials and the table prefix out of `brama.yaml`. (ADR-0005, ADR-0009, ADR-0014)
