# Fixture Conventions

> 最后更新：2026-05-03（移除显式 Memory 设计）

`fixtures/` contains human-readable scenario fixtures. `testdata/` contains
stable inputs and golden outputs for automated tests.

Fixtures should model current AVM behavior only:

- Agent/Profile YAML
- Environment YAML
- registry capability metadata
- adapter render plans
- runtime layout samples

Do not add memory fixtures unless a new memory design is accepted and
implemented.
