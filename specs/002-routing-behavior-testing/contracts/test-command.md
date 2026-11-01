# Test Command Contract

## Invocation

```text
istio-route test [-f <file|->]... [--manifests <name>=<file>]... [--junit <file>] <feature-file|directory>...
```

`test` is a child command. Existing invocations such as
`istio-route shop.example.com` remain hostname traces and retain their existing
flag and exit-code behavior.

## Inputs

- `-f`, `--file`: append a file to one unnamed manifest set. `-` reads
  multi-document YAML/JSON from stdin.
- `--manifests name=file`: append a file to a named set. Repeat the same name to
  add more files to that set.
- A run may use one unnamed set or one or more named sets, not a mixture.
- Standard input may appear at most once in the entire manifest input.
- Positional feature inputs accept files or directories. Directories are searched
  recursively for `.feature` files in deterministic order.
- At least one manifest set and at least one runnable feature scenario are required.
- No cluster flags or cluster access are supported by this command.

## Environment selection

- Each supplied named set is evaluated in a separate suite execution.
- A scenario tagged with a supplied environment name runs only for that environment.
- Untagged scenarios run once for each supplied set.
- Other tags do not select an environment.
- `Given the manifests for "name"` confirms that scenario's current set. It cannot
  switch environments. It is invalid when the current set differs or the named set
  was not supplied.

## Output streams

- Human-readable feature/scenario results and assertion failures go to stdout.
- Input, parse, step-definition, and report-write diagnostics go to stderr.
- A failure includes its feature location and `expected X, got Y` with actual trace
  information.
- `--junit file` writes one combined JUnit XML report with one testcase per scenario
  execution, including outline rows and environment executions.
- Failure to write the report is an invalid-input/output failure.

## Exit statuses

| Status | Meaning for `istio-route test` |
|---|---|
| 0 | All executed scenario assertions passed |
| 1 | At least one scenario assertion failed |
| 2 | Invalid usage, feature, unknown/ambiguous step, unreadable input, or report error |

Invalid input/output status 2 takes precedence over assertion-failure status 1.
These meanings apply only to the `test` subcommand. Existing hostname trace statuses
remain 0 success, 1 runtime error, 2 usage error, and 3 hostname not found.
