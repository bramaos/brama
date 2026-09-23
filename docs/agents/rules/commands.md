# Command rules

What brama decided about Cobra commands in `internal/cli`. The reason is where each rule
points; `docs/adr/` is `ADR-NNNN`, `docs/product-description.md` is `PD`.
A rule with no citation is uncited on purpose: its reason is the code it describes.

- Keep `RunE` to reading flags, args and the working directory, then calling `run<Command>` with everything as parameters, fakeable dependencies included. A command that only renders a constant, like `version`, needs no `run<Command>`.
- Pass `cmd.Context()` as the first parameter of any `run<Command>` that can block; wrap it (`signal.NotifyContext`) rather than start from `context.Background()`.
- Return errors from `RunE`; never call `os.Exit`, `log.Fatal` or `panic`. `Main` alone turns the outcome into exit 0, 1 or 42. (`internal/cli/cli.go` package doc)
- Return a `refusal.Refusal` when brama is not allowed to act and an error when something broke; a Refusal is not a failure. (ADR-0003, PD §10)
- Let `--non-interactive` and `--json` stop prompts, never guardrails; add no `--yes` or `--force` that skips a check. (PD §10, ADR-0005)
- Ask only through `env.Ask`, and treat a nil `Ask` as the normal case: take the narrower answer and finish. (`console.Ask` doc)
- Add no destructive remote operation to v0.1; deployment waits for the data half to ship. (ADR-0004)
