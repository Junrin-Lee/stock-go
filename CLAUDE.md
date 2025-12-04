# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Stock Monitor** is a professional command-line TUI (Terminal User Interface) application for real-time stock price tracking, portfolio management, and watchlist analysis. Built with Go using the Bubble Tea framework, it supports A-shares (Shanghai/Shenzhen), US stocks, and Hong Kong stocks with bilingual support (Chinese/English).

**Current Version**: v4.9 - AI-generated project with enhanced intraday charts (smart date selection, adaptive Y-axis), multi-tag system, and user experience optimizations

## Essential Commands

### Build & Run
```bash
# Build the executable
go build -o cmd/stock-monitor

# Run directly without building
go run main.go

# Run the compiled binary
./cmd/stock-monitor
```

### Dependencies
```bash
# Download all dependencies
go mod download

# Install core dependencies manually (if needed)
go get github.com/charmbracelet/bubbletea@v1.3.6
go get github.com/jedib0t/go-pretty/v6@v6.6.8
go get golang.org/x/text@v0.28.0
go get gopkg.in/yaml.v3@v3.0.1
```

### Testing & Debugging
- **No formal test suite**: Project uses manual testing
- **Debug Mode**: Enable in `cmd/conf/config.yml` (`debug_mode: true`), then press 'd' in app to view logs
- **Test Directory**: `/test` folder exists but is empty (reserved for future use)

## Architecture Overview

### Core Application Pattern
This is a **state machine-based TUI application** with clear separation of concerns:

```
┌─────────────────────────────────────────┐
│   UI Layer (Bubble Tea Framework)      │
│   - Event Loop, View Rendering          │
├─────────────────────────────────────────┤
│   Application Logic (main.go)           │
│   - 13 States, Data Models, Business    │
├─────────────────────────────────────────┤
│   Data Persistence (JSON + YAML)        │
│   - portfolio.json, watchlist.json      │
├─────────────────────────────────────────┤
│   External APIs (HTTP)                   │
│   - Tencent, Finnhub, Sina, East Money  │
└─────────────────────────────────────────┘
```

### Key Modules

| File | Lines | Purpose |
|------|-------|---------|
| **main.go** | ~6,424 | Core application: state machine, TUI event handling, all business logic, intraday chart visualization |
| **intraday.go** | ~469 | Background intraday data collection with worker pool (max 10 concurrent) |
| **sort.go** | ~238 | Sorting engine for 11 portfolio fields and 7 watchlist fields |
| **color.go** | ~55 | Color utilities using go-pretty (red/green/white for gains/losses/neutral) |
| **consts.go** | ~71 | Application constants (states, sort fields, file paths, enums) |

### State Machine (13 States)
The application flows through these states:
- `MainMenu` → Entry point with menu navigation
- `AddingStock`, `EditingStock` → Stock management
- `Monitoring` → Portfolio real-time monitoring (main view)
- `WatchlistViewing` → Watchlist with tag filtering
- `SearchingStock`, `SearchResult`, `SearchResultWithActions` → Stock search flow
- `WatchlistTagging`, `WatchlistTagSelect`, `WatchlistTagManage`, `WatchlistTagEdit` → Tag management
- `PortfolioSorting`, `WatchlistSorting` → Sorting configuration
- `LanguageSelection` → Language switching

Each state has a dedicated handler function: `handleMainMenu()`, `handleMonitoring()`, `handleAddingStock()`, etc.

### Data Flow Pattern (Bubble Tea)

```
User Input (tea.KeyMsg)
    → Update() method (routes to state handler)
    → State handler modifies Model
    → Returns updated Model + optional tea.Cmd
    → View() renders new state
    → Terminal updates

Async Operations:
Timer Tick (5s)
    → startStockPriceUpdates() (spawns goroutines)
    → fetchStockPrice() API calls
    → stockPriceUpdateMsg sent back
    → Cache updated (with RWMutex)
    → View() renders fresh data
```

## Critical Architectural Patterns

### 1. **Concurrency & Thread Safety**
- Uses `sync.RWMutex` for stock price cache (concurrent reads, exclusive writes)
- Goroutine-based async stock price fetching every 5 seconds
- Worker pool pattern in `intraday.go` with buffered channel semaphore (max 10 workers)
- **Important**: Always protect shared state with proper locking

### 2. **Bubble Tea MVC-like Pattern**
- **Model**: `main.Model` struct (~232 fields) contains ALL application state
- **View**: `View()` method switches on `state` field to render different screens
- **Controller**: `Update(msg tea.Msg)` method handles all events (keyboard, ticks, custom messages)
- **Commands**: Long-running operations return `tea.Cmd` functions for async execution

### 3. **Multi-API Fallback Strategy**
Stock data fetching has automatic fallback:
- **A-shares**: Tencent API (primary) → Sina API (fallback)
- **US/HK stocks**: Finnhub API
- **Search**: Tencent Search → Sina Search → keyword transformation → TwelveData API
- Always display "-" when data unavailable (never fake data)

### 4. **Real-Time Sorting with Live Updates**
- Sorting is **not cached** - re-applied on each update with latest prices
- Sort state preserved during auto-refresh (field + direction)
- Uses `DefaultSorter` interface implementation with `sort.Slice`

### 5. **Configuration-Driven Behavior**
- YAML config at `cmd/conf/config.yml` auto-created on first run
- Runtime config changes (language, debug mode) auto-saved
- Internationalization via `i18n/zh.json` and `i18n/en.json`

## Directory Structure

```
stock-go/
├── main.go                  # Core application (6,424 lines)
├── intraday.go             # Intraday data collection (469 lines)
├── sort.go                 # Sorting engine (238 lines)
├── color.go                # Color utilities (55 lines)
├── consts.go               # Constants (71 lines)
├── go.mod / go.sum         # Go module dependencies
│
├── cmd/
│   ├── stock-monitor       # Compiled executable
│   └── conf/
│       ├── config.yml      # User configuration (YAML, auto-generated)
│       └── config_demo.yaml # Config template
│
├── data/
│   ├── portfolio.json      # Held stocks (auto-generated)
│   ├── watchlist.json      # Watched stocks with tags (auto-generated)
│   └── intraday/           # Intraday minute-by-minute data (organized by code/date)
│       ├── SH600000/       # Stock code directory
│       │   ├── 20251202.json  # Date-specific intraday data
│       │   └── 20251203.json
│       └── SZ000001/
│           └── 20251202.json
│
├── i18n/
│   ├── zh.json            # Chinese translations (~250 strings)
│   └── en.json            # English translations (~250 strings)
│
├── doc/
│   ├── issues/
│   │   ├── plans/         # Plan Agent generated implementation plans
│   │   ├── INTRADAY_FEATURE.md  # Intraday data collection documentation
│   │   └── INTRADAY_CHART_IMPLEMENTATION_PLAN.md
│   └── version/           # Version history documentation
│
├── README.md              # Chinese documentation
└── README_EN.md           # English documentation
```

## Key Data Structures

### Stock Models
```go
// Portfolio stock with holdings
type Stock struct {
    Code, Name string
    Price, CostPrice float64
    Quantity int
    Change, ChangePercent float64
    StartPrice, MaxPrice, MinPrice, PrevClose float64
}

// Real-time market data (from API)
type StockData struct {
    Symbol, Name string
    Price, Change, ChangePercent float64
    StartPrice, MaxPrice, MinPrice, PrevClose float64
    TurnoverRate float64
    Volume int64
}

// Watchlist stock with multi-tag support
type WatchlistStock struct {
    Code, Name string
    Tags []string  // Multiple tags per stock
}
```

### Application State
```go
type Model struct {
    // State machine
    state AppState  // One of 13 states

    // Data
    portfolio Portfolio
    watchlist Watchlist
    config Config

    // Cache (protected by RWMutex)
    stockPriceCache map[string]*StockPriceCacheEntry
    stockPriceCacheMutex sync.RWMutex

    // Background data collection
    intradayManager *IntradayManager

    // UI state (~200+ fields for cursors, inputs, filters, etc.)
}
```

## Stock Code Format

The application auto-converts between different formats:

| Market | Input Format | API Format | Example |
|--------|--------------|------------|---------|
| Shanghai A-share | `SH601138` or `601138` | `601138.SS` | 工业富联 |
| Shenzhen A-share | `SZ000001` or `000001` | `000001.SZ` | 平安银行 |
| US Stock | `AAPL` | `AAPL` | Apple |
| Hong Kong | `HK00700` or `0700.HK` | `0700.HK` | Tencent |

**Character Encoding**: Uses `golang.org/x/text/encoding/simplifiedchinese` for GBK ↔ UTF-8 conversion (Chinese API responses use GBK).

## Intraday Data Collection (Background Feature)

- **Trigger**: Automatically starts when entering Monitoring or WatchlistViewing states
- **Worker Pool**: Max 10 concurrent goroutines with buffered channel semaphore
- **Update Frequency**: Every 1 minute during trading hours (09:30-11:30, 13:00-15:00)
- **Data Storage**: `data/intraday/{CODE}/YYYYMMDD.json` with minute-by-minute prices
- **APIs**: Sina Finance (primary) → East Money (fallback)
- **Data Format**: JSON with code, name, date, datapoints array (time + price), and update timestamp
- **Persistence**: Data is permanently retained, organized by stock code and date directories
- **Thread Safety**: File-level locks via sync.Map, atomic writes using temp files + rename
- See `doc/issues/INTRADAY_FEATURE.md` for detailed documentation

## Intraday Chart Visualization (v4.9)

The application provides terminal-based intraday charts using the `ntcharts` library (Braille characters for smooth rendering).

### Key Features

1. **Smart Date Selection** (`getSmartChartDate`):
   - Before market open (< 9:30): Shows previous trading day's data
   - During/after trading (≥ 9:30): Shows current day's data
   - Automatically skips weekends when finding previous trading day

2. **Adaptive Y-Axis Margin** (`calculateAdaptiveMargin`):
   - Low volatility (< 1%): 50% margin for better visual detail
   - Medium volatility (1-3%): 20% margin
   - High volatility (> 3%): 10% margin
   - Ensures minimum 0.3% margin for edge cases

3. **Fixed Time Framework** (`createFixedTimeRange`):
   - Creates complete 9:30-15:00 timeline (331 data points)
   - Includes lunch break period for correct time mapping
   - Missing data points filled with last known price

4. **Chart Rendering**:
   - Uses `github.com/NimbleMarkets/ntcharts/linechart` + `canvas`
   - Braille character drawing (`DrawBrailleLineWithStyle`) for smooth curves
   - Custom X-axis labels showing rounded times (whole/half hours)
   - Color coding: Green (up), Red (down), White (neutral)

### Access Methods

- Press `V` in Monitoring (portfolio) or WatchlistViewing state
- Displays stock code, name, date, and trading session markers

### Technical Details

```go
// Time point structure for chart data
type TimePoint struct {
    Time  time.Time
    Value float64
}

// Smart date selection logic
func getSmartChartDate() string {
    now := time.Now()
    if now.Hour() < 9 || (now.Hour() == 9 && now.Minute() < 30) {
        return findPreviousTradingDayFromDate(now.Format("20060102"))
    }
    return now.Format("20060102")
}
```

## Configuration System

### Config File Location
`cmd/conf/config.yml` - Auto-created with defaults on first run

### Key Settings
```yaml
system:
  language: zh                  # "zh" or "en"
  auto_start: true             # Jump to monitoring if data exists
  startup_module: portfolio     # "portfolio" or "watchlist"
  debug_mode: false            # Show debug logs (press 'd' in app)

display:
  color_scheme: professional   # Color display mode
  decimal_places: 3            # Price precision
  table_style: light           # Table rendering style
  max_lines: 10                # Rows per page
  portfolio_highlight: yellow  # Highlight color for owned stocks in watchlist

update:
  refresh_interval: 5          # Data refresh seconds
  auto_update: true            # Enable auto-refresh
```

## Claude Code Workflow

### Plan Agent Documentation

When using the Plan Agent (via EnterPlanMode tool) to design implementation plans for complex features:

- **Storage Location**: All plan documents MUST be saved to `./doc/issues/plans/` directory
- **File Naming**: Use descriptive names with uppercase and underscores (e.g., `FEATURE_NAME_PLAN.md`)
- **Purpose**: Centralized location for all implementation plans to maintain project organization and enable context integration

### Creating the Directory

The `doc/issues/plans/` directory should be created if it doesn't exist when the first plan is generated.

## Development Considerations

### When Adding New Features

1. **State Management**:
   - Add new state to `AppState` enum in `consts.go`
   - Create handler function `handleYourNewState()` in `main.go`
   - Update `Update()` method to route to your handler
   - Update `View()` method to render your state

2. **API Integration**:
   - Follow existing fallback pattern (primary API → backup API)
   - Always handle failures gracefully (display "-" for missing data)
   - Use timeout (8-10 seconds) to prevent hangs
   - Consider GBK encoding for Chinese data sources

3. **Concurrency**:
   - Use `sync.RWMutex` for read-heavy shared data
   - Use worker pools (buffered channels) for bounded parallelism
   - Never mutate Model state from goroutines - send messages via `tea.Cmd`

4. **Internationalization**:
   - Add entries to both `i18n/zh.json` and `i18n/en.json`
   - Use `m.getI18nText(key)` helper to retrieve translations

5. **Data Persistence**:
   - Use JSON for structured data (portfolio, watchlist)
   - Use YAML for configuration
   - Always validate before saving, handle errors gracefully

### Performance Optimization Tips

- **Caching**: Stock price cache prevents redundant API calls during rendering
- **Pagination**: Use `display.max_lines` to limit rendered rows
- **Sorting**: Sorting happens on-demand, not on every render
- **API Calls**: Batched every 5 seconds, not per-stock

### Common Pitfalls

1. **Character Width Calculation**: Chinese characters are 2 cells wide - use `go-pretty` library functions for proper alignment
2. **State Mutations**: Never modify Model state directly from goroutines - always use message passing
3. **API Failures**: Always provide fallback behavior - never let API failures crash the app
4. **Sort State**: Remember that sorting must be re-applied after data updates for real-time accuracy

## File Encoding Notes

- **Source Code**: UTF-8
- **API Responses**: GBK (Chinese APIs) - converted to UTF-8
- **Config Files**: UTF-8 (YAML, JSON)
- **Terminal Output**: UTF-8 with ANSI color codes

## Version History

- **v4.9** (Current): Enhanced intraday charts with smart date selection, adaptive Y-axis margin, fixed time framework, Braille rendering
- **v4.8**: Multi-tag system, portfolio highlighting in watchlist, cursor editing, sorting optimizations
- **v4.7**: Architecture optimization, internationalization enhancements
- **v4.6**: Intraday data collection, async optimizations
- **v4.5**: Advanced sorting system (11 portfolio fields, 7 watchlist fields)
- See `doc/version/` for complete history

## Important Notes for Future Development

1. **main.go is monolithic** (~6,424 lines) - this is intentional for simplicity. Consider refactoring if adding major features.
2. **No database** - uses JSON files. For >1000 stocks, consider migrating to SQLite.
3. **No tests** - relies on manual testing. Consider adding unit tests for critical business logic.
4. **Hardcoded trading hours** - A-shares trading hours are hardcoded in `intraday.go`. Add to config if supporting more markets.
5. **API keys not required** - current APIs are public. If adding paid APIs, add key management to config.
6. **Intraday data growth** - Intraday data accumulates over time (~10KB/stock/day). Consider implementing data cleanup or compression for long-term use.
