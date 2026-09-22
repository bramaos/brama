# Architecture rules

What brama decided about where code lives and what it may do. The reason is where each
rule points; `docs/adr/` is `ADR-NNNN`, `docs/product-description.md` is `PD`.
A rule with no citation is uncited on purpose: its reason is the code it describes.

- Keep the core free of `internal/renderer`, `internal/cli`, Cobra and every Charm package. (PD §9, "The rendering rule")
- Add each new core package to `corePackages` in `internal/arch/arch_test.go`, which enforces the rule above.
- Keep `cmd/brama/main.go` to `os.Exit(cli.Main(version))`; commands and exit codes live in `internal/cli`. `cmd/brama-shim` links only the core.
- Never assign a package-level `var` after init; pass anything a test would swap, as `console` is passed.
- Anonymize on the Server, inside the Shim; never let a raw value of a `fake` or `drop` column reach the laptop. (ADR-0001)
- Roll fake values at random per Pull, never derived from the real value; keep the mapping on the Server and destroy it when the Pull ends. (ADR-0002)
- Refuse, exit 42, while any column or Discriminator value is Unclassified; never default it, warn about it or skip it. (ADR-0003, ADR-0012)
- Keep the Generator vocabulary closed, in `internal/generator`; an Adapter supplies only `fake.password`. (ADR-0013)
- Read a Schema from the database catalogue: `information_schema` on MySQL, `pg_catalog` on PostgreSQL. (ADR-0009, ADR-0010 pg_catalog)
- Read the table prefix from the project on every run; never record it or fall back to the framework default. (ADR-0014)
