# AGENTS.md

## Build & Run
```bash
go build -o cmd/stock-monitor    # Build executable
go run main.go                   # Run directly
./cmd/stock-monitor              # Run compiled binary
go mod download                  # Install dependencies
```

## Testing
No test suite exists. Manual testing only. Debug mode: set `debug_mode: true` in `cmd/conf/config.yml`, press 'd' in app.

## Code Style
- **Imports**: Standard library first (alphabetical), then third-party. Use aliases like `tea "github.com/charmbracelet/bubbletea"`
- **Naming**: PascalCase for exported types/funcs, camelCase for private. JSON/YAML tags use snake_case
- **Formatting**: Use `gofmt`. Tabs for indentation
- **Error handling**: Early return on errors. Display "-" for missing data. Use `debugPrint()` for debug logs
- **Concurrency**: Use `sync.RWMutex` for caches, worker pools with buffered channels (max 10), never mutate Model from goroutines—use `tea.Cmd` messages
- **Comments**: Chinese for business logic, English for technical docs

## Architecture
Single `main` package. Bubble Tea MVC pattern with 13 states (see `consts.go`). Core logic in `main.go` (~6k lines). Supporting: `intraday.go`, `sort.go`, `color.go`, `consts.go`.
