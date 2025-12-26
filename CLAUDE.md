# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **Note**: This is an AI-generated repository. The entire project was created by AI, including code architecture, implementation, and documentation.

## Project Overview

**Stock Monitor** is a professional command-line TUI (Terminal User Interface) application for real-time stock price tracking, portfolio management, and watchlist analysis. Built with Go using the Bubble Tea framework, it supports A-shares (Shanghai/Shenzhen), US stocks, and Hong Kong stocks with bilingual support (Chinese/English).

**Current Version**: v5.7 - Tag grouping system with separated market/user tags, cursor position memory, and enhanced UX. Includes v5.6's search view integration with real-time charts, v5.5's market tag system, and v5.4's HK stock turnover rate fix

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
- **Test Suite**: Growing test coverage with `api_test.go` (API fallback logic, code conversion) and `intraday_test.go` (market detection, collection modes)
- **Run Tests**: `go test -v ./` to run all tests
- **Debug Mode**: Enable in `cmd/conf/config.yml` (`debug_mode: true`), then press 'd' in app to view logs

## Architecture Overview

### Core Application Pattern
This is a **state machine-based TUI application** with clear separation of concerns:

```
┌─────────────────────────────────────────┐
│   UI Layer (Bubble Tea Framework)      │
│   - main.go, ui_utils.go, format.go    │
│   - Event Loop, View Rendering          │
├─────────────────────────────────────────┤
│   Application Logic                     │
│   - main.go (19 States, Orchestration)  │
│   - watchlist.go, intraday_chart.go     │
│   - sort.go (Data Models, Business)     │
├─────────────────────────────────────────┤
│   Data Layer                            │
│   - persistence.go (JSON/YAML I/O)      │
│   - cache.go (In-Memory Cache)          │
│   - types.go (Data Structures)          │
├─────────────────────────────────────────┤
│   External Integration                  │
│   - api.go (Multi-API Fallback)         │
│   - intraday.go (Background Collector)  │
│   - Tencent, Finnhub, Sina, East Money  │
└─────────────────────────────────────────┘
```

### Key Modules

| File | Lines | Purpose |
|------|-------|---------|
| **main.go** | ~2,961 | Core application: state machine, TUI event handling, orchestration |
| **intraday.go** | ~1,436 | Background intraday data collection with intelligent worker pool, multi-market support, auto-stop logic (v5.3) |
| **api.go** | ~1,355 | External API integration: Tencent, Sina, Finnhub, TwelveData, East Money with fallback logic (v5.4+) |
| **intraday_chart.go** | ~894 | Intraday chart visualization: smart date selection, adaptive Y-axis, Braille rendering, timezone support (v5.3) |
| **watchlist.go** | ~559 | Watchlist management: tag operations, market labels, filtering, search, group selection (v5.5) |
| **columns.go** | ~492 | Column metadata system: configurable columns for portfolio and watchlist (v5.2) |
| **persistence.go** | ~353 | Data persistence layer: JSON/YAML read/write, file operations, backup/restore, legacy migration, market tag migration (v5.5) |
| **types.go** | ~321 | Data structure definitions: Stock, StockData, WatchlistStock with Market field, WorkerMetadata, CollectionMode, TradingState, MarketType (v5.5) |
| **sort.go** | ~238 | Sorting engine for 11 portfolio fields and 7 watchlist fields |
| **ui_utils.go** | ~194 | UI rendering utilities: table formatting, pagination, Chinese character width handling |
| **timezone.go** | ~172 | Timezone handling: market-specific timezone conversions, trading state detection, multi-market support (v5.3) |
| **debug.go** | ~160 | Debug logging: 1000-entry buffer, scrollable viewer, conditional i18n logging |
| **format.go** | ~156 | Formatting utilities: number formatting, price display, percentage calculations |
| **cache.go** | ~127 | Stock price caching: proper Bubble Tea async pattern, 30-second TTL, RWMutex protection (v5.3) |
| **api_test.go** | ~87 | Unit tests: API fallback logic, code conversion functions, HK stock detection (v5.4) |
| **intraday_test.go** | ~116 | Unit tests: Market detection, data collection modes, worker management (v5.3+) |
| **scroll.go** | ~77 | Scroll handling: cursor management, pagination logic |
| **consts.go** | ~71 | Application constants (states, sort fields, file paths, enums) |
| **i18n.go** | ~70 | Internationalization: translation loading, language switching, fallback logic |
| **color.go** | ~55 | Color utilities using go-pretty (red/green/white for gains/losses/neutral) |

### State Machine (19 States)
The application flows through these states:

**Core Navigation:**
1. `MainMenu` → Entry point with menu navigation
2. `LanguageSelection` → Language switching

**Stock Management:**
3. `AddingStock` → Add new stock to portfolio
4. `EditingStock` → Edit existing stock in portfolio
5. `SearchingStock` → Search for stock information
6. `SearchResult` → Display search results
7. `SearchResultWithActions` → Search result with action options

**Portfolio & Watchlist:**
8. `Monitoring` → Portfolio real-time monitoring (main view)
9. `WatchlistViewing` → Watchlist with tag filtering
10. `WatchlistSearchConfirm` → Confirm adding stock to watchlist from search

**Tag Management:**
11. `WatchlistTagging` → Tag a watchlist stock
12. `WatchlistTagSelect` → Select tags for a stock
13. `WatchlistTagManage` → Manage tags (view all tags for a stock)
14. `WatchlistTagRemoveSelect` → Select tags to remove from a stock
15. `WatchlistTagEdit` → Edit tag name
16. `WatchlistGroupSelect` → Select watchlist group/filter

**Sorting & Visualization:**
17. `PortfolioSorting` → Configure portfolio sorting
18. `WatchlistSorting` → Configure watchlist sorting
19. `IntradayChartViewing` → View intraday price chart

Each state has a dedicated handler function: `handleMainMenu()`, `handleMonitoring()`, `handleAddingStock()`, `handleWatchlistTagging()`, etc.

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

## Architectural Evolution to v5.0+ Modular Design

The codebase has undergone significant architectural evolution culminating in v5.0's complete modularization and v5.1's multi-market enhancements:

**Evolution Path:**
- **Early versions**: Monolithic single main.go (~6,400+ lines) with all logic intertwined
- **v4.x (initial refactoring)**: Started extracting functionality into separate modules
- **v5.0**: Complete modular architecture - 16 focused, independently maintainable modules
  - **Result**: 50% reduction in main.go size (~6,400 → ~3,150 lines), zero functional loss, backward-compatible upgrade
  - **Benefit**: Clearer code organization, easier to locate features, simpler to add new functionality
- **v5.1 (current)**: Extended architecture with timezone-aware multi-market support - added timezone.go (17 modules total)
  - **Result**: Fixed US/HK stock intraday data collection, implemented market-specific timezone handling
  - **New Pattern**: Smart API routing based on market type with 3-layer fallback strategy

**Modular Architecture Benefits:**

1. **Separation of Concerns**
   - API integration isolated in `api.go` (1,226 lines)
   - Data persistence in `persistence.go` (171 lines)
   - UI utilities in dedicated modules (`ui_utils.go`, `format.go`, `scroll.go`)
   - Business logic remains in `main.go` but focused on state machine

2. **Improved Testability**
   - Smaller, focused modules are easier to unit test
   - Clear module boundaries reduce coupling
   - API fallback logic can be tested independently

3. **Better Maintainability**
   - Related functionality grouped together (e.g., all watchlist operations in `watchlist.go`)
   - Easier navigation and code discovery
   - Reduced cognitive load when making changes

4. **Enhanced Reusability**
   - Utility modules (`format.go`, `color.go`, `cache.go`) can be reused across features
   - Type definitions centralized in `types.go`
   - Debug infrastructure in `debug.go` available application-wide

**Key Architectural Layers:**

```
┌─────────────────────────────────────────┐
│   UI Layer (Bubble Tea Framework)      │
│   - main.go (state machine, events)    │
│   - ui_utils.go (rendering helpers)    │
│   - format.go (display formatting)     │
│   - scroll.go (pagination)             │
├─────────────────────────────────────────┤
│   Business Logic Layer                  │
│   - watchlist.go (watchlist ops)       │
│   - intraday_chart.go (visualization)  │
│   - sort.go (sorting engine)           │
├─────────────────────────────────────────┤
│   Data Layer                            │
│   - cache.go (in-memory caching)       │
│   - persistence.go (JSON/YAML I/O)     │
│   - types.go (data structures)         │
├─────────────────────────────────────────┤
│   External Integration Layer            │
│   - api.go (stock data APIs)           │
│   - intraday.go (background collector) │
├─────────────────────────────────────────┤
│   Cross-Cutting Concerns               │
│   - timezone.go (market-specific TZ)   │
│   - i18n.go (internationalization)     │
│   - debug.go (logging)                 │
│   - color.go (theming)                 │
│   - consts.go (configuration)          │
└─────────────────────────────────────────┘
```

This modular architecture provides a solid foundation for future features while maintaining the simplicity and directness that characterize the project.

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
- **A-shares**: Tencent API (primary) → Sina API (fallback) → East Money
- **US/HK stocks**: Finnhub API → Yahoo Finance → East Money (for HK turnover rate補充, v5.4)
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
stock-monitor/
├── main.go                  # Core application: state machine, event handling (~2,961 lines)
├── api.go                   # External API integration with fallback logic (~1,355 lines, includes East Money API)
├── intraday.go             # Background intraday data collection (~1,436 lines)
├── intraday_chart.go       # Intraday chart visualization (~894 lines)
├── watchlist.go            # Watchlist management, tag operations, market labels (~559 lines)
├── columns.go              # Column metadata system (~492 lines)
├── persistence.go          # Data persistence layer, market tag migration (~353 lines)
├── types.go                # Data structure definitions (~321 lines, includes MarketType)
├── sort.go                 # Sorting engine (~238 lines)
├── ui_utils.go             # UI rendering utilities (~194 lines)
├── timezone.go             # Timezone handling (~172 lines)
├── debug.go                # Debug logging system (~160 lines)
├── format.go               # Formatting utilities (~156 lines)
├── cache.go                # Stock price caching (~127 lines)
├── api_test.go             # Unit tests for API functions (~87 lines, v5.4)
├── intraday_test.go        # Unit tests for intraday collection (~116 lines)
├── scroll.go               # Scroll handling (~77 lines)
├── consts.go               # Application constants (~71 lines)
├── i18n.go                 # Internationalization (~70 lines)
├── color.go                # Color utilities (~55 lines)
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
│   └── changelogs/        # Version history documentation
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

// Watchlist stock with multi-tag support and market identification
type WatchlistStock struct {
    Code   string
    Name   string
    Tags   []string    // User-defined tags only (market tags separated since v5.5)
    Market MarketType  // Auto-detected market type: china/us/hongkong (v5.5)
}
```

### Application State
```go
type Model struct {
    // State machine
    state AppState  // One of 19 states

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
- **Multi-Market Support** (v5.1):
  - **A-shares**: Tencent Finance (primary) → East Money (fallback) → Sina Finance
  - **US stocks**: Yahoo Finance (primary) → fallbacks
  - **HK stocks**: Tencent Finance (primary) → Yahoo Finance → East Money
  - **Market Detection**: Automatic based on stock code format (SH/SZ, AAPL, HK prefix)
  - **Timezone Handling**: timezone.go module converts UTC to market-specific timezones
- **Update Frequency**: Every 1 minute during market-specific trading hours
- **Data Storage**: `data/intraday/{CODE}/YYYYMMDD.json` with minute-by-minute prices
- **Data Format**: JSON with code, name, date, datapoints array (time + price), and update timestamp
- **Persistence**: Data is permanently retained, organized by stock code and date directories
- **Thread Safety**: File-level locks via sync.Map, atomic writes using temp files + rename
- See `doc/issues/INTRADAY_FEATURE.md` for detailed documentation

## Intraday Chart Visualization (v5.1)

The application provides terminal-based intraday charts using the `ntcharts` library (Braille characters for smooth rendering). v5.1 adds timezone-aware display for multi-market support.

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

**Recent Major Versions:**
- **v5.7** (Current): 🏷️ Tag grouping system - separated market/user tag groups, cursor position memory, boundary stop behavior, enhanced group selection UI
- **v5.6**: 🔍 Search view integration - real-time intraday charts in search results, 5-second auto-refresh, temporary data collection (non-persistent), ESC/Q key support
- **v5.5**: 🏷️ Market tag system - automatic market detection (A-share/US/HK), market labels in UI, data migration for legacy tags, bilingual support, Market field separation from Tags
- **v5.4**: 🔧 HK stock turnover rate fix - East Money API integration as fallback for HK turnover data, unit tests added (api_test.go with 3 test functions)
- **v5.3**: 🛡️ Critical fixes and intelligent enhancements - watchlist deadlock fix, intelligent worker metadata tracking, three-mode collection strategy (Historical/Live/Complete), trading state detection (5 states), auto-stop logic, market-specific expected datapoints (240/390/330)
- **v5.2**: 📊 Customizable table columns - 14+12 configurable columns, metadata-driven architecture, simple/detailed/custom modes
- **v5.1**: 🌍 Multi-market support with timezone-aware data collection, fixes for US/HK intraday data acquisition, improved chart color logic, new timezone.go module for market-specific handling
- **v5.0**: 🏗️ Architecture modernization - Complete modular design (16 modules), main.go 50% smaller (~6,400 → ~3,150 lines), three-tier architecture
- **v4.9**: Enhanced intraday charts with smart date selection, adaptive Y-axis margin, fixed time framework, Braille rendering
- **v4.8**: Multi-tag system, portfolio highlighting in watchlist, cursor editing, sorting optimizations
- **v4.7**: Architecture optimization, internationalization enhancements
- **v4.6**: Intraday data collection, async optimizations
- **v4.5**: Advanced sorting system (11 portfolio fields, 7 watchlist fields)

See `doc/changelogs/README.md` for complete version history, and `doc/changelogs/v5.7.md`, `doc/changelogs/v5.6.md`, `doc/changelogs/v5.5.md` for recent detailed documentation

## Quick Reference: Where to Find Things

When modifying the codebase, use this guide to quickly locate what you need:

| Task | File(s) | Notes |
|------|---------|-------|
| **Add new state/view** | `main.go`, `consts.go` | Define AppState in consts.go, implement handler in main.go |
| **Modify data display** | `ui_utils.go`, `format.go` | Table formatting in ui_utils.go, number formatting in format.go |
| **Add API or fix data fetching** | `api.go` | Multi-API fallback already implemented, just add new API call |
| **Manage stock prices/cache** | `cache.go` | RWMutex-protected cache with 30-second TTL |
| **Save/load user data** | `persistence.go` | JSON for portfolio/watchlist, YAML for config |
| **Add UI colors/themes** | `color.go` | Color utilities using go-pretty library |
| **Add translations** | `i18n/zh.json`, `i18n/en.json` | Keep both files in sync |
| **Market-specific timezone/hours** | `timezone.go` | Market detection, trading hours, timezone conversions (v5.1) |
| **Background tasks** | `intraday.go` | Worker pool pattern with max 10 concurrent goroutines, multi-market support |
| **Stock data structures** | `types.go` | Central location for Stock, StockData, Config types |
| **Debug/logging** | `debug.go` | Accessible via debug mode, press 'd' in app |
| **Sorting logic** | `sort.go` | Implements DefaultSorter interface for portfolio/watchlist |
| **Watchlist operations** | `watchlist.go` | Multi-tag management, filtering, grouping |
| **Pagination/scrolling** | `scroll.go` | Handles cursor and pagination logic |

## Important Notes for Future Development

1. **v5.5 Modular architecture** - 20 focused modules (17 core + 2 test files + consts) with clear responsibilities. Main.go (~2,961 lines, further reduced from v5.3's ~3,150) is purely an orchestration layer. New market tag system (v5.5) separates market identification from user tags. timezone.go module handles market-specific timezone conversions and trading hours detection. Current design is near-optimal for project size.

2. **Data Storage** - Uses JSON files (portfolio.json, watchlist.json). For >1000 stocks or complex queries, consider migrating to SQLite in v6.0. v5.5 includes automatic data migration for legacy market tags.

3. **Testing approach** - Growing test coverage with api_test.go (API fallback logic, code conversion, v5.4) and intraday_test.go (market detection, collection modes). Run tests with `go test -v ./`. Continue expanding tests with cache.go (TTL/concurrency) as it's complex and least coupled to UI.

4. **Hardcoded trading hours** - A-shares trading hours (09:30-11:30, 13:00-15:00) hardcoded in intraday.go. If adding more markets, move to config.yml.

5. **Public APIs only** - Current APIs don't require keys. If adding paid API services, implement API key management in config system.

6. **Intraday data growth** - Data accumulates (~10KB/stock/day). Monitor `data/intraday/` size; implement cleanup or compression for long-term deployments.

7. **Character encoding** - Chinese API responses use GBK encoding. Always use `golang.org/x/text/encoding/simplifiedchinese` for conversions.