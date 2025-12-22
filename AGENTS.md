# AGENTS.md

Quick reference for AI coding agents. See **CLAUDE.md** for full architecture details.

## Build & Test
```bash
go build -o cmd/stock-monitor    # Build
go run main.go                   # Run directly
go test ./...                    # Run all tests
go test -v -run TestConvertStockCodeForTencent ./  # Single test (use -run with test name pattern)
gofmt -w . && go vet ./...       # Format and lint
```

## Code Style
- **Imports**: stdlib first (alphabetical), then third-party with aliases (e.g., `tea "github.com/charmbracelet/bubbletea"`)
- **Formatting**: `gofmt` with tabs (not spaces); Chinese comments for business logic, English for technical docs
- **Naming**: Exported=PascalCase, private=camelCase, JSON/YAML tags=snake_case
- **Errors**: Early return on errors, display "-" for missing data, use `debugPrint()` for debug logs
- **Concurrency**: Use `sync.RWMutex` for shared state; never mutate Model from goroutines—send `tea.Cmd` messages instead
- **Encoding**: UTF-8 source; GBK→UTF-8 conversion for Chinese API responses (see `api.go`)

## Key Files
| Task | File |
|------|------|
| State machine & handlers | `main.go` |
| API calls (multi-fallback) | `api.go` |
| Data types | `types.go` |
| Tests (reference pattern) | `intraday_test.go` |
| Translations (keep in sync) | `i18n/zh.json`, `i18n/en.json` |

## Plan Mode Agent Protocol
**IMPORTANT**: When executing plans from plan mode agent:
1. **First step**: Save plan document to `./doc/` directory BEFORE implementation
   - Feature plans → `./doc/plans/`
   - Bug fixes → `./doc/issues/`
2. Only proceed with implementation after documentation is saved
