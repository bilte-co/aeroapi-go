# aeroapi-go

A Go client library for the [FlightAware AeroAPI](https://www.flightaware.com/aeroapi/), auto-generated from the OpenAPI specification using [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen).

- **Module path:** `github.com/bilte-co/aeroapi-go`
- **AeroAPI version:** 4.28.0

## Installation

```bash
go get github.com/bilte-co/aeroapi-go@latest
```

## Usage

```go
package main

import (
    "context"
    "net/http"

    "github.com/bilte-co/aeroapi-go"
)

func main() {
    c, err := client.NewClientWithResponses(
        "https://aeroapi.flightaware.com/aeroapi",
        client.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
            req.Header.Set("x-apikey", "YOUR_API_KEY")
            return nil
        }),
    )
    if err != nil {
        panic(err)
    }

    // Use c to call AeroAPI endpoints
    _ = c
}
```

## Compatibility

| aeroapi-go version | FlightAware AeroAPI version |
|--------------------|-----------------------------|
| v0.1.0             | 4.28.0                      |

## Development

### Prerequisites

- Go 1.24+
- [just](https://github.com/casey/just) (command runner)

### Commands

| Command        | Description                              |
|----------------|------------------------------------------|
| `just gen`     | Regenerate client from OpenAPI spec      |
| `just test`    | Run tests                                |
| `just lint`    | Run golangci-lint                        |
| `just sec`     | Run security checks (gosec)              |
| `just vuln`    | Run vulnerability checks (govulncheck)   |
| `just check`   | Run tests, lint, vuln (pre-release)      |
| `just release` | Tag a new release (e.g., `just release 0.1.0`) |

### Regenerating the Client

The client is generated from the FlightAware AeroAPI OpenAPI specification v4.28.0.

```bash
just gen
# or
go generate ./...
```

## License

See [LICENSE](LICENSE) for details.
