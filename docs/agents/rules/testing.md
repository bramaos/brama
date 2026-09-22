# Testing rules

`docs/agents/go.md` §Testing is the contract: stdlib `testing` only, table-driven subtests
where cases share logic, `t.Context()`, no `t.Parallel()`. These are what brama adds.

- Test a command through `run<Command>` with `testEnv()` (or a JSON `console` over a `bytes.Buffer`) and fakes for its dependencies, never through `Main` or the real stdout.
- Check JSON output by decoding it, and a Refusal through `refusal.As`, never by matching text.
- Test anything that dials a Server against the containerised rig under `test/testenv`, and store no credential for it. (ADR-0008)
- Keep the seed's planted Unclassified column; it is what drives the Refusal path. (ADR-0008)
