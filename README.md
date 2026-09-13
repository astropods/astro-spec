<div align="center">

# astro-spec

**Parser and type definitions for the Astro AI agent specification (`astropods.yml`).**

[![Go Reference](https://pkg.go.dev/badge/github.com/astropods/astro-spec.svg)](https://pkg.go.dev/github.com/astropods/astro-spec)
[![Go Version](https://img.shields.io/badge/go-1.24-00ADD8?logo=go&logoColor=white)](go.mod)

[Spec Reference](https://docs.astropods.com/astropods-package-spec) · [Installation](#installation) · [Usage](#usage) · [The Spec](#the-spec) · [API](#api-surface)

</div>

---

## Overview

`astro-spec` is the single source of truth for how an agent's declarative
configuration is parsed and validated across the Astro platform. It is consumed
as a shared Go module by both:

- **`astro-cli`** — builds, pushes, and registers agents, and
- **`astro-server`** — deploys them to the cluster,

so the two always agree on the meaning of an `astropods.yml`. The package also
emits a JSON Schema for editor autocomplete and validation.

> 📖 For the complete field-by-field specification, see the
> [**Astro Package Spec reference**](https://docs.astropods.com/astropods-package-spec).

## Installation

```sh
go get github.com/astropods/astro-spec
```

```go
import spec "github.com/astropods/astro-spec"
```

Requires **Go 1.24+**.

## Usage

Parse an `astropods.yml` from disk:

```go
s, err := spec.ParseFile("astropods.yml")
if err != nil {
    log.Fatalf("invalid spec: %v", err)
}
fmt.Println(s.Name, "→", s.Agent.Image)
```

| Function                | Input            |
|-------------------------|------------------|
| `Parse(data []byte)`    | in-memory bytes  |
| `ParseString(s string)` | in-memory string |
| `ParseFile(path)`       | file on disk     |
| `ParseSpec(path)`       | file on disk     |

Every entry point decodes and then validates, so a returned spec has passed
the same checks regardless of how it was read. `Parse` is the only
implementation; the other three delegate to it.

To validate a spec you already hold, for example one decoded from stored
JSON, call the checks directly:

| Function                      | Reports                                      |
|-------------------------------|----------------------------------------------|
| `Validate(s) error`           | the first problem, or nil                    |
| `Problems(s) []Problem`       | every problem, in a deterministic order      |

`Validate` is a wrapper over `Problems`, so the two cannot disagree. Each
`Problem` carries the dotted YAML `Field` it concerns alongside its
`Message`, so a caller can attribute it to a location in the document.

These checks cover what the spec format itself defines. Anything that needs
outside context stays with the caller: whether a provider names a real
backend, whether a credential has a value, whether a schedule expression
parses, or whether a runtime is permitted in a given environment.

## The Spec

An `AstroSpec` describes one agent and its supporting components:

| Field          | Purpose                                            |
|----------------|----------------------------------------------------|
| `spec`         | Spec version — must be `blueprint/v1`              |
| `name`         | Unique agent name                                  |
| `meta`         | **Deprecated — omit.** `description`/`tags` moved to the Agent Card (v1.2); `visibility` to the platform UI/API (v1.5) |
| `agent`        | The main agent container (image or build)          |
| `models`       | Model sidecar containers                           |
| `knowledge`    | Knowledge store containers                         |
| `integrations` | Integration sidecar containers                     |
| `providers`    | Custom provider definitions                        |
| `inputs`       | User-supplied inputs injected into every container |
| `ingestion`    | Data ingestion pipelines                           |
| `dev`          | Local development overrides                        |

### Example

```yaml
spec: blueprint/v1
name: support-agent

agent:
  image: ghcr.io/acme/support-agent:latest
  interfaces:
    frontend: true
    messaging: true

models:
  primary:
    provider: anthropic
  backup:
    provider: openai

knowledge:
  docs:
    provider: qdrant

integrations:
  github:
    provider: github

ingestion:
  docs-sync:
    container:
      image: acme/docs-ingest:latest
      environment:
        SOURCE_REPO: acme/handbook
    trigger:
      type: schedule
```

Components can reference one another with `${...}` references (for example, an
agent reading a knowledge store's connection URL).

## API Surface

| Area              | Functions                                                                                     |
|-------------------|-----------------------------------------------------------------------------------------------|
| **Parsing**       | `Parse`, `ParseString`, `ParseFile`, `ParseSpec`                                              |
| **Validation**    | `Validate`, `Problems`, `ValidateName`, `ValidateVarName`, `SecretDefaultViolations`, `DeprecationWarnings` |
| **Env resolution**| `ResolveEnvVars`, `AllCredentialKeys`, `AgentConnectionKeys`, and related credential-key helpers |
| **Providers**     | `LookupBuiltin`, `GetProvider`, `IsCloudModelProvider`, `IsGatewayModelProvider`, `CredentialKeys` |
| **JSON Schema**   | `Schema()` — returns the embedded JSON Schema                                                  |

## Evaluation sets

`eval` (`github.com/astropods/astro-spec/eval`) parses and validates
`EVALUATION.yaml`, the document a builder uses to define a custom
per-agent evaluation set:

```go
import evalspec "github.com/astropods/astro-spec/eval"

result, err := evalspec.Parse(yamlText, promptFiles)
```

`Parse` checks structure and preset-reference names offline; it never
resolves a preset ref to its prompt text.

## JSON Schema

`astropods.schema.json` is generated by reflecting over the Go types.
Regenerate it after changing any spec type:

```sh
go generate ./...
# equivalently:
go run ./cmd/generate-schema
```

## Development

```sh
go test ./...
```

When you change a spec type, keep `astropods.schema.json` in sync by
regenerating it (see [JSON Schema](#json-schema)).

## Contributing

Contributions are welcome. Please open an issue or pull request. Before
submitting, make sure the tests pass (`go test ./...`) and, if you touched any
spec type, that the JSON Schema has been regenerated.

## License

Part of the [Astro AI](https://docs.astropods.com) platform. Licensed under the
[Apache License 2.0](LICENSE) — Copyright 2026 Postman Inc.
