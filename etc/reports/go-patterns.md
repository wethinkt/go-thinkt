# Research Report: Go Project Patterns

**Date**: 2026-01-24
**Sources**:
- https://github.com/NimbleMarkets/dbn-go
- https://github.com/AgentDank/screentime-mcp

## CLI Framework

### spf13/cobra + pflag

Standard pattern from dbn-go:

```go
import "github.com/spf13/cobra"

// Global flags at package level
var (
    verbose bool
    outputFile string
)

// Root command
var rootCmd = &cobra.Command{
    Use:   "tool-name",
    Short: "Brief description",
    Long:  `Longer description with examples.`,
}

// Subcommands as vars
var subCmd = &cobra.Command{
    Use:   "subcommand [args]",
    Short: "What it does",
    Args:  cobra.MinimumNArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        // implementation
    },
}

func main() {
    // Register subcommands
    rootCmd.AddCommand(subCmd)

    // Attach flags
    rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
    subCmd.Flags().StringVarP(&outputFile, "output", "o", "", "output file")

    rootCmd.Execute()
}
```

## Project Structure

```
project/
├── cmd/
│   └── tool-name/
│       └── main.go          # Cobra CLI, all in one file for simple tools
├── internal/
│   ├── module1/             # Private packages
│   └── module2/
├── tests/                   # Integration tests, fixtures
├── .github/workflows/       # CI/CD
├── Taskfile.yml             # Build automation
├── go.mod
├── go.sum
└── .goreleaser.yaml         # Multi-platform releases
```

## Taskfile Patterns

From dbn-go:

```yaml
version: '3'

vars:
  BIN_DIR: ./bin

tasks:
  default:
    deps: [test, build]

  build:
    desc: Build all binaries
    deps: [go-tidy]
    cmds:
      - go build -o {{.BIN_DIR}}/toolname ./cmd/toolname

  test:
    desc: Run tests
    cmds:
      - go test ./...

  go-tidy:
    desc: Tidy modules
    cmds:
      - go mod tidy

  clean:
    desc: Remove build artifacts
    cmds:
      - rm -rf {{.BIN_DIR}}
```

## Coding Conventions

1. **Error handling**: Continue processing on error, report to stderr
2. **Flag patterns**: `BoolVarP`, `StringVarP` with short flags
3. **Environment fallback**: Check env vars when flags not provided
4. **Args validation**: Use `cobra.MinimumNArgs()`, `cobra.ExactArgs()`

## Dependencies of Note

- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/pflag` - Flag parsing (via cobra)
- `github.com/charmbracelet/*` - TUI components (optional)

## Module Naming

Pattern: `github.com/<org>/<project>`
- `github.com/NimbleMarkets/dbn-go`
- `github.com/AgentDank/screentime-mcp`
- Proposed: `github.com/Brain-STM-org/thinking-tracer-tools`
