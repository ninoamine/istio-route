# Gherkin Step Language Contract

## Document structure

The parser accepts standard Gherkin `Feature`, `Background`, `Scenario`,
`Scenario Outline`, `Examples`, and tags. Each expanded example is one scenario
execution. Step keywords may use standard Gherkin synonyms (`Given`, `When`,
`Then`, `And`, `But`) where supported by Godog.

The language is closed: only the step patterns in the
[step vocabulary reference](../step-reference.md) are defined. No user-defined
step code is loaded. A pattern must have one unambiguous interpretation; undefined
or ambiguous steps are invalid and include the closest supported pattern when
possible.

## Execution context

Each environment execution has one immutable manifest set and zero or more trace
results. A manifest-set assertion can confirm but cannot change that set. A hostname
trace step stores the structured analysis for subsequent assertion steps.
Background steps run before each scenario under the same execution context.

## Assertion semantics

- Resource, entry-point, route, and rule positions are one-based and follow the
  stable order exposed in the trace.
- String field equality is exact unless a specific step documents otherwise.
- A positive assertion over unavailable or absent information fails with an
  expected/actual explanation; it is not silently treated as a match.
- A no-route result is valid structured trace data and can satisfy a not-found
  assertion.
- Percentages and destination weights are asserted against the values in the trace;
  no traffic is sent.
- Finding severity is one of `info`, `warning`, or `error`.

The vocabulary is a user-facing contract. Adding or removing patterns requires
updating the reference and its conformance tests.
