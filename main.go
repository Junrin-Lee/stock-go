package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"gopkg.in/yaml.v3"
)

type Stock struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	CostPrice     float64 `json:"cost_price"`
	Quantity      int     `json:"quantity"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	StartPrice    float64 `json:"start_price"`
	MaxPrice      float64 `json:"max_price"`
	MinPrice      float64 `json:"min_price"`
	PrevClose     float64 `json:"prev_close"`
}

type StockData struct {
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Price         float64 `json:"price"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
	StartPrice    float64 `json:"start_price"`
	MaxPrice      float64 `json:"max_price"`
	MinPrice      float64 `json:"min_price"`
	PrevClose     float64 `json:"prev_close"` // 昨日收盘价
	TurnoverRate  float64 `json:"turnover_rate"`
	Volume        int64   `json:"volume"`
}

type Portfolio struct {
	Stocks []Stock `json:"stocks"`
}

// 自选股票数据结构
type WatchlistStock struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type Watchlist struct {
	Stocks []WatchlistStock `json:"stocks"`
}

// 系统配置结构
type Config struct {
	// 系统设置
	System SystemConfig `yaml:"system"`
	// 显示设置
	Display DisplayConfig `yaml:"display"`
	// 更新设置
	Update UpdateConfig `yaml:"update"`
}

type SystemConfig struct {
	Language      string `yaml:"language"`       // 默认语言 "zh" 或 "en"
	AutoStart     bool   `yaml:"auto_start"`     // 有数据时自动进入监控模式
	StartupModule string `yaml:"startup_module"` // 启动模块 "portfolio"(持股) 或 "watchlist"(自选)
	DebugMode     bool   `yaml:"debug_mode"`     // 调试模式开关
}

type DisplayConfig struct {
	ColorScheme   string `yaml:"color_scheme"`   // 颜色方案 "professional", "simple"
	DecimalPlaces int    `yaml:"decimal_places"` // 价格显示小数位数
	TableStyle    string `yaml:"table_style"`    // 表格样式 "light", "bold", "simple"
	MaxLines      int    `yaml:"max_lines"`      // 列表每页最大显示行数
}

type UpdateConfig struct {
	RefreshInterval int  `yaml:"refresh_interval"` // 刷新间隔（秒）
	AutoUpdate      bool `yaml:"auto_update"`      // 是否自动更新
}

const (
	dataFile        = "data/portfolio.json"
	watchlistFile   = "data/watchlist.json"
	configFile      = "cmd/conf/config.yml"
	refreshInterval = 5 * time.Second
)

type Language string

const (
	Chinese Language = "zh"
	English Language = "en"
)

type AppState int

const (
	MainMenu AppState = iota
	AddingStock
	RemovingStock
	Monitoring
	EditingStock
	SearchingStock
	SearchResult
	LanguageSelection
	WatchlistViewing
	WatchlistRemoving
	SearchResultWithActions
	WatchlistSearchConfirm
)

// 文本映射结构
type TextMap map[string]string

// 语言文本映射
var texts = map[Language]TextMap{
	Chinese: {
		"title":               "=== 股票监控系统 ===",
		"stockList":           "持股列表",
		"watchlist":           "自选股票",
		"stockSearch":         "股票搜索",
		"addStock":            "添加股票",
		"editStock":           "修改股票",
		"removeStock":         "删除股票",
		"debugMode":           "调试模式",
		"language":            "语言",
		"exit":                "退出",
		"on":                  "开启",
		"off":                 "关闭",
		"chinese":             "中文",
		"english":             "English",
		"keyHelp":             "使用方向键 ↑↓ 或 W/S 键选择，回车/空格确认，Q键退出",
		"keyHelpWin":          "使用 W/S 键选择，回车确认，Q键退出",
		"returnToMenu":        "ESC、Q键或M键返回主菜单",
		"returnToMenuShort":   "ESC或Q键返回主菜单",
		"returnEscOnly":       "ESC键返回",
		"holdingsHelp":        "ESC、Q键或M键返回主菜单，E键修改股票，D键删除股票，A键添加股票 | ↑/↓:翻页",
		"watchlistHelp":       "ESC、Q键或M键返回主菜单，D键删除股票，A键添加股票 | ↑/↓:翻页",
		"monitoringTitle":     "=== 股票实时监控 ===",
		"updateTime":          "更新时间(5s): %s",
		"emptyPortfolio":      "投资组合为空",
		"addStockFirst":       "请先添加股票到投资组合",
		"total":               "总计",
		"addingTitle":         "=== 添加股票 ===",
		"enterCode":           "请输入股票代码: ",
		"enterCost":           "请输入成本价: ",
		"enterQuantity":       "请输入股票数量: ",
		"codeFormat":          "支持格式: SH601138, 000001, AAPL 等",
		"stockCode":           "股票代码: %s",
		"stockName":           "股票名称: %s",
		"currentPrice":        "当前价格: %.3f",
		"openPrice":           "开盘价",
		"highPrice":           "最高价",
		"lowPrice":            "最低价",
		"prevClose":           "昨收价",
		"change":              "涨跌",
		"costPrice":           "成本价: %s",
		"codeRequired":        "股票代码不能为空",
		"costRequired":        "成本价不能为空",
		"quantityRequired":    "数量不能为空",
		"invalidPrice":        "无效的价格格式",
		"invalidQuantity":     "无效的数量格式",
		"fetchingInfo":        "正在获取股票信息...",
		"stockNotFound":       "无法获取股票 %s 的信息，请检查股票代码是否正确",
		"addSuccess":          "成功添加股票: %s (%s)",
		"removeTitle":         "=== 删除股票 ===",
		"selectToRemove":      "选择要删除的股票:",
		"navHelp":             "使用方向键选择，回车确认，ESC或Q键返回",
		"removeSuccess":       "成功删除股票: %s (%s)",
		"editTitle":           "=== 修改股票 ===",
		"selectToEdit":        "选择要修改的股票:",
		"currentCost":         "当前成本价: %.3f",
		"enterNewCost":        "请输入新的成本价: ",
		"newCost":             "新成本价: %.3f",
		"currentQuantity":     "当前数量: %d",
		"enterNewQuantity":    "请输入新的数量: ",
		"editSuccess":         "成功修改股票 %s 的成本价和数量",
		"searchTitle":         "=== 股票搜索 ===",
		"enterSearch":         "请输入股票代码或名称: ",
		"searchFormats":       "支持格式:\n• 中文名称: 贵州茅台, 苹果, 腾讯, 阿里巴巴 等\n• 中国股票: SH601138, 000001, SZ000002 等\n• 美股: AAPL, TSLA, MSFT 等\n• 港股: HK00700 等\n\n💡 提示: 中文检索成功率较低，建议优先使用股票代码检索",
		"searchHelp":          "回车搜索，ESC键返回主菜单",
		"searching":           "正在搜索股票信息...",
		"searchNotFound":      "无法找到股票 %s 的信息，请检查输入是否正确",
		"detailTitle":         "=== 股票详情信息 ===",
		"noInfo":              "未找到股票信息",
		"detailHelp":          "ESC或Q键返回主菜单，R键重新搜索",
		"emptyCannotEdit":     "投资组合为空，无法修改股票",
		"languageTitle":       "=== 语言选择 ===",
		"selectLanguage":      "请选择您的语言:",
		"languageHelp":        "使用方向键选择，回车确认，ESC或Q键返回主菜单",
		"watchlistTitle":      "=== 自选实时监控 ===",
		"emptyWatchlist":      "自选列表为空",
		"addToWatchFirst":     "请先添加股票到自选列表",
		"removeFromWatch":     "从自选列表删除",
		"selectToRemoveWatch": "选择要从自选列表删除的股票:",
		"removeWatchSuccess":  "成功从自选列表删除股票: %s (%s)",
		"addToWatchlist":      "添加到自选",
		"addToPortfolio":      "添加到持股列表",
		"addWatchSuccess":     "成功添加到自选列表: %s (%s)",
		"alreadyInWatch":      "股票 %s 已在自选列表中",
		"actionHelp":          "1-添加到自选, 2-添加到持股列表, ESC或Q键返回主菜单, R键重新搜索",
	},
	English: {
		"title":               "=== Stock Monitor System ===",
		"stockList":           "Holdings",
		"watchlist":           "Watchlist",
		"stockSearch":         "Stock Search",
		"addStock":            "Add Stock",
		"editStock":           "Edit Stock",
		"removeStock":         "Remove Stock",
		"debugMode":           "Debug Mode",
		"language":            "Language",
		"exit":                "Exit",
		"on":                  "On",
		"off":                 "Off",
		"chinese":             "中文",
		"english":             "English",
		"keyHelp":             "Use arrow keys ↑↓ or W/S to select, Enter/Space to confirm, Q to exit",
		"keyHelpWin":          "Use W/S keys to select, Enter to confirm, Q to exit",
		"returnToMenu":        "ESC, Q or M to return to main menu",
		"returnToMenuShort":   "ESC or Q to return to main menu",
		"returnEscOnly":       "ESC to return",
		"holdingsHelp":        "ESC, Q or M to return to main menu, E to edit stock, D to delete stock, A to add stock | ↑/↓:scroll",
		"watchlistHelp":       "ESC, Q or M to return to main menu, D to delete stock, A to add stock | ↑/↓:scroll",
		"monitoringTitle":     "=== Real-time Stock Monitor ===",
		"updateTime":          "Update Time(5s): %s",
		"emptyPortfolio":      "Portfolio is empty",
		"addStockFirst":       "Please add stocks to your portfolio first",
		"total":               "Total",
		"addingTitle":         "=== Add Stock ===",
		"enterCode":           "Enter stock code: ",
		"enterCost":           "Enter cost price: ",
		"enterQuantity":       "Enter quantity: ",
		"codeFormat":          "Supported formats: SH601138, 000001, AAPL, etc.",
		"stockCode":           "Stock Code: %s",
		"stockName":           "Stock Name: %s",
		"currentPrice":        "Current Price: %.3f",
		"openPrice":           "Open Price",
		"highPrice":           "High Price",
		"lowPrice":            "Low Price",
		"prevClose":           "Prev Close",
		"change":              "Change",
		"costPrice":           "Cost Price: %s",
		"codeRequired":        "Stock code cannot be empty",
		"costRequired":        "Cost price cannot be empty",
		"quantityRequired":    "Quantity cannot be empty",
		"invalidPrice":        "Invalid price format",
		"invalidQuantity":     "Invalid quantity format",
		"fetchingInfo":        "Fetching stock information...",
		"stockNotFound":       "Unable to get information for stock %s, please check the code is correct",
		"addSuccess":          "Successfully added stock: %s (%s)",
		"removeTitle":         "=== Remove Stock ===",
		"selectToRemove":      "Select stock to remove:",
		"navHelp":             "Use arrow keys to select, Enter to confirm, ESC or Q to return",
		"removeSuccess":       "Successfully removed stock: %s (%s)",
		"editTitle":           "=== Edit Stock ===",
		"selectToEdit":        "Select stock to edit:",
		"currentCost":         "Current cost price: %.3f",
		"enterNewCost":        "Enter new cost price: ",
		"newCost":             "New cost price: %.3f",
		"currentQuantity":     "Current quantity: %d",
		"enterNewQuantity":    "Enter new quantity: ",
		"editSuccess":         "Successfully edited stock %s cost price and quantity",
		"searchTitle":         "=== Stock Search ===",
		"enterSearch":         "Enter stock code or name: ",
		"searchFormats":       "Supported formats:\n• Chinese names: 贵州茅台, Apple, Tencent, Alibaba, etc.\n• Chinese stocks: SH601138, 000001, SZ000002, etc.\n• US stocks: AAPL, TSLA, MSFT, etc.\n• Hong Kong stocks: HK00700, etc.\n\n💡 Tip: Chinese name searches have lower success rates, recommend using stock codes",
		"searchHelp":          "Press Enter to search, ESC to return to main menu",
		"searching":           "Searching stock information...",
		"searchNotFound":      "Unable to find information for stock %s, please check your input is correct",
		"detailTitle":         "=== Stock Detail Information ===",
		"noInfo":              "No stock information found",
		"detailHelp":          "ESC or Q to return to main menu, R to search again",
		"emptyCannotEdit":     "Portfolio is empty, cannot edit stocks",
		"languageTitle":       "=== Language Selection ===",
		"selectLanguage":      "Please select your language:",
		"languageHelp":        "Use arrow keys to select, Enter to confirm, ESC or Q to return to main menu",
		"watchlistTitle":      "=== Real-time Stock Monitor ===",
		"emptyWatchlist":      "Watchlist is empty",
		"addToWatchFirst":     "Please add stocks to your watchlist first",
		"removeFromWatch":     "Remove from Watchlist",
		"selectToRemoveWatch": "Select stock to remove from watchlist:",
		"removeWatchSuccess":  "Successfully removed stock from watchlist: %s (%s)",
		"addToWatchlist":      "Add to Watchlist",
		"addToPortfolio":      "Add to Holdings",
		"addWatchSuccess":     "Successfully added to watchlist: %s (%s)",
		"alreadyInWatch":      "Stock %s is already in watchlist",
		"actionHelp":          "1-Add to Watchlist, 2-Add to Holdings, ESC or Q to return to main menu, R to search again",
	},
}

type Model struct {
	state           AppState
	currentMenuItem int
	menuItems       []string
	cursor          int
	input           string
	message         string
	portfolio       Portfolio
	watchlist       Watchlist // 自选股票列表
	config          Config    // 系统配置
	debugMode       bool
	language        Language
	debugLogs       []string // 调试日志存储
	debugScrollPos  int      // debug日志滚动位置

	// For stock addition
	addingStep    int
	tempCode      string
	tempCost      string
	tempQuantity  string
	stockInfo     *StockData
	fromSearch    bool     // 标记是否从搜索结果添加
	previousState AppState // 记录进入编辑/删除前的状态

	// For stock editing
	editingStep        int
	selectedStockIndex int

	// For stock searching
	searchInput         string
	searchResult        *StockData
	searchFromWatchlist bool // 标记是否从自选列表进入搜索

	// For language selection
	languageCursor int

	// For monitoring
	lastUpdate time.Time

	// For scrolling
	portfolioScrollPos int // 持股列表滚动位置
	watchlistScrollPos int // 自选列表滚动位置
	portfolioCursor    int // 持股列表当前选中行
	watchlistCursor    int // 自选列表当前选中行
}

type tickMsg struct{}

// 获取本地化文本的辅助函数
func (m *Model) getText(key string) string {
	if text, exists := texts[m.language][key]; exists {
		return text
	}
	// 如果找不到文本，返回英文版本作为备用
	if text, exists := texts[English][key]; exists {
		return text
	}
	return key // 最后备用返回key本身
}

// 获取菜单项
func (m *Model) getMenuItems() []string {
	return []string{
		m.getText("stockList"),
		m.getText("watchlist"),
		m.getText("stockSearch"),
		m.getText("debugMode"),
		m.getText("language"),
		m.getText("exit"),
	}
}

func main() {
	// 确保目录存在
	os.MkdirAll("data", 0755)
	os.MkdirAll("cmd/conf", 0755)

	// 加载配置文件
	config := loadConfig()
	portfolio := loadPortfolio()
	watchlist := loadWatchlist()

	// 根据配置和是否有股票数据决定初始状态
	initialState := MainMenu
	var lastUpdate time.Time
	if config.System.AutoStart {
		// 根据startup_module配置决定启动哪个模块
		switch config.System.StartupModule {
		case "portfolio":
			// 启动持股模块，需要有持股数据
			if len(portfolio.Stocks) > 0 {
				initialState = Monitoring
				lastUpdate = time.Now()
			}
		case "watchlist":
			// 启动自选模块，需要有自选数据
			if len(watchlist.Stocks) > 0 {
				initialState = WatchlistViewing
				lastUpdate = time.Now()
			}
		default:
			// 默认行为：如果有持股数据则进入持股模块
			if len(portfolio.Stocks) > 0 {
				initialState = Monitoring
				lastUpdate = time.Now()
			}
		}
	}

	// 根据配置文件设置语言
	language := English // 默认英文
	if config.System.Language == "zh" {
		language = Chinese
	}

	m := Model{
		state:              initialState,
		currentMenuItem:    0,
		portfolio:          portfolio,
		watchlist:          watchlist,
		config:             config,
		debugMode:          config.System.DebugMode,
		language:           language,
		lastUpdate:         lastUpdate,
		debugLogs:          make([]string, 0),
		debugScrollPos:     0, // 初始滚动位置
		portfolioScrollPos: 0, // 持股列表滚动位置
		watchlistScrollPos: 0, // 自选列表滚动位置
		portfolioCursor:    0, // 持股列表游标
		watchlistCursor:    0, // 自选列表游标
	}

	// 根据语言设置菜单项
	m.menuItems = m.getMenuItems()

	// 设置全局模型引用用于调试日志
	globalModel = &m

	p := tea.NewProgram(&m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func (m *Model) Init() tea.Cmd {
	if m.state == Monitoring || m.state == WatchlistViewing {
		return m.tickCmd()
	}
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var newModel tea.Model
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// debug滚动快捷键，在任何状态下都可用
		if m.debugMode {
			keyStr := msg.String()

			switch keyStr {
			case "pgup":
				m.scrollDebugUp()
				return m, nil
			case "pgdown":
				m.scrollDebugDown()
				return m, nil
			case "home":
				m.scrollDebugToTop()
				return m, nil
			case "end":
				m.scrollDebugToBottom()
				return m, nil
			}
		}

		// 持股列表和自选列表滚动快捷键
		if m.state == Monitoring || m.state == WatchlistViewing {
			keyStr := msg.String()
			switch keyStr {
			case "up":
				if m.state == Monitoring {
					m.scrollPortfolioUp()
				} else {
					m.scrollWatchlistUp()
				}
				return m, nil
			case "down":
				if m.state == Monitoring {
					m.scrollPortfolioDown()
				} else {
					m.scrollWatchlistDown()
				}
				return m, nil
			}
		}

		// 处理各状态的正常按键
		switch m.state {
		case MainMenu:
			newModel, cmd = m.handleMainMenu(msg)
		case AddingStock:
			newModel, cmd = m.handleAddingStock(msg)
		case RemovingStock:
			newModel, cmd = m.handleRemovingStock(msg)
		case Monitoring:
			newModel, cmd = m.handleMonitoring(msg)
		case EditingStock:
			newModel, cmd = m.handleEditingStock(msg)
		case SearchingStock:
			newModel, cmd = m.handleSearchingStock(msg)
		case SearchResult:
			newModel, cmd = m.handleSearchResult(msg)
		case SearchResultWithActions:
			newModel, cmd = m.handleSearchResultWithActions(msg)
		case WatchlistSearchConfirm:
			newModel, cmd = m.handleWatchlistSearchConfirm(msg)
		case LanguageSelection:
			newModel, cmd = m.handleLanguageSelection(msg)
		case WatchlistViewing:
			newModel, cmd = m.handleWatchlistViewing(msg)
		case WatchlistRemoving:
			newModel, cmd = m.handleWatchlistRemoving(msg)
		default:
			newModel, cmd = m, nil
		}
	case tickMsg:
		if m.state == Monitoring || m.state == WatchlistViewing {
			m.lastUpdate = time.Now()
			newModel, cmd = m, m.tickCmd()
		} else {
			newModel, cmd = m, nil
		}
	default:
		newModel, cmd = m, nil
	}

	// 更新全局模型引用以保持调试日志同步
	if newModel != nil {
		if modelPtr, ok := newModel.(*Model); ok {
			globalModel = modelPtr
		}
	}

	return newModel, cmd
}

func (m *Model) View() string {
	var mainContent string
	switch m.state {
	case MainMenu:
		mainContent = m.viewMainMenu()
	case AddingStock:
		mainContent = m.viewAddingStock()
	case RemovingStock:
		mainContent = m.viewRemovingStock()
	case Monitoring:
		mainContent = m.viewMonitoring()
	case EditingStock:
		mainContent = m.viewEditingStock()
	case SearchingStock:
		mainContent = m.viewSearchingStock()
	case SearchResult:
		mainContent = m.viewSearchResult()
	case SearchResultWithActions:
		mainContent = m.viewSearchResultWithActions()
	case WatchlistSearchConfirm:
		mainContent = m.viewWatchlistSearchConfirm()
	case LanguageSelection:
		mainContent = m.viewLanguageSelection()
	case WatchlistViewing:
		mainContent = m.viewWatchlistViewing()
	case WatchlistRemoving:
		mainContent = m.viewWatchlistRemoving()
	default:
		mainContent = ""
	}

	// 添加调试面板
	return mainContent + m.renderDebugPanel()
}

func (m *Model) handleMainMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k", "w":
		if m.currentMenuItem > 0 {
			m.currentMenuItem--
		}
		m.message = "" // 清除消息
	case "down", "j", "s":
		if m.currentMenuItem < len(m.menuItems)-1 {
			m.currentMenuItem++
		}
		m.message = "" // 清除消息
	case "enter", " ":
		return m.executeMenuItem()
	case "q", "ctrl+c":
		m.savePortfolio()
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) executeMenuItem() (tea.Model, tea.Cmd) {
	m.message = "" // 清除之前的消息
	switch m.currentMenuItem {
	case 0: // 股票列表
		m.logUserAction("进入持股监控页面")
		m.state = Monitoring
		// 设置滚动位置和光标到显示前N条股票
		if len(m.portfolio.Stocks) > 0 {
			maxPortfolioLines := m.config.Display.MaxLines
			if len(m.portfolio.Stocks) > maxPortfolioLines {
				// 显示前N条：滚动位置设置为显示从索引0开始的N条
				m.portfolioScrollPos = len(m.portfolio.Stocks) - maxPortfolioLines
				m.portfolioCursor = 0 // 光标指向第一个股票（索引0）
			} else {
				// 股票数量不超过显示行数，显示全部
				m.portfolioScrollPos = 0
				m.portfolioCursor = 0
			}
		}
		m.lastUpdate = time.Now()
		return m, m.tickCmd()
	case 1: // 自选股票
		m.logUserAction("进入自选股票页面")
		m.state = WatchlistViewing
		// 设置滚动位置和光标到显示前N条股票
		if len(m.watchlist.Stocks) > 0 {
			maxWatchlistLines := m.config.Display.MaxLines
			if len(m.watchlist.Stocks) > maxWatchlistLines {
				// 显示前N条：滚动位置设置为显示从索引0开始的N条
				m.watchlistScrollPos = len(m.watchlist.Stocks) - maxWatchlistLines
				m.watchlistCursor = 0 // 光标指向第一个股票（索引0）
			} else {
				// 股票数量不超过显示行数，显示全部
				m.watchlistScrollPos = 0
				m.watchlistCursor = 0
			}
		}
		m.cursor = 0
		m.message = ""
		m.lastUpdate = time.Now()
		return m, m.tickCmd()
	case 2: // 股票搜索
		m.logUserAction("进入股票搜索页面")
		m.state = SearchingStock
		m.searchInput = ""
		m.searchResult = nil
		m.searchFromWatchlist = false
		m.message = ""
		return m, nil
	case 3: // 调试模式
		if m.debugMode {
			m.logUserAction("关闭调试模式")
		} else {
			m.logUserAction("开启调试模式")
		}
		m.debugMode = !m.debugMode
		m.config.System.DebugMode = m.debugMode
		// 保存配置到文件
		if err := saveConfig(m.config); err != nil && m.debugMode {
			m.message = fmt.Sprintf("Warning: Failed to save config: %v", err)
		}
		return m, nil
	case 4: // 语言选择页面
		m.logUserAction("进入语言选择页面")
		m.state = LanguageSelection
		m.languageCursor = 0
		if m.language == English {
			m.languageCursor = 1
		}
		return m, nil
	case 5: // 退出
		m.logUserAction("用户退出程序")
		m.savePortfolio()
		m.saveWatchlist()
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) viewMainMenu() string {
	s := m.getText("title") + "\n\n"

	for i, item := range m.menuItems {
		prefix := "  "
		if i == m.currentMenuItem {
			prefix = "► "
		}

		if i == 3 { // 调试模式
			debugStatus := m.getText("off")
			if m.debugMode {
				debugStatus = m.getText("on")
			}
			s += fmt.Sprintf("%s%s: %s\n", prefix, item, debugStatus)
		} else if i == 4 { // 语言选择
			langStatus := m.getText("english")
			if m.language == Chinese {
				langStatus = m.getText("chinese")
			}
			s += fmt.Sprintf("%s%s: %s\n", prefix, item, langStatus)
		} else {
			s += fmt.Sprintf("%s%s\n", prefix, item)
		}
	}

	s += "\n"
	if runtime.GOOS == "windows" {
		s += m.getText("keyHelpWin") + "\n"
	} else {
		s += m.getText("keyHelp") + "\n"
	}
	s += "=========================\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) handleAddingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// 根据来源决定返回目标
		if m.fromSearch {
			// 从持股列表或搜索结果进入，返回相应页面
			if m.previousState == Monitoring {
				m.state = Monitoring
				m.lastUpdate = time.Now()
			} else {
				m.state = SearchResultWithActions
			}
			m.fromSearch = false // 重置标志
		} else {
			m.state = MainMenu
		}
		m.message = ""
		return m, nil
	case "enter":
		return m.processAddingStep()
	case "backspace":
		if len(m.input) > 0 {
			// 正确处理多字节字符（如中文）的删除
			runes := []rune(m.input)
			if len(runes) > 0 {
				m.input = string(runes[:len(runes)-1])
			}
		}
	default:
		// 改进的输入处理：支持多字节字符（如中文）
		str := msg.String()
		if len(str) > 0 && str != "\n" && str != "\r" && !isControlKey(str) {
			m.input += str
		}
	}
	return m, nil
}

func (m *Model) processAddingStep() (tea.Model, tea.Cmd) {
	switch m.addingStep {
	case 0: // 搜索股票
		if m.input == "" {
			m.message = m.getText("codeRequired")
			return m, nil
		}
		m.message = m.getText("searching")

		// 使用搜索功能
		var stockData *StockData
		if containsChineseChars(m.input) {
			stockData = searchChineseStock(m.input)
		} else {
			stockData = getStockPrice(m.input)
		}

		if stockData == nil || stockData.Name == "" {
			m.message = fmt.Sprintf(m.getText("searchNotFound"), m.input)
			m.input = ""
			return m, nil
		}

		// 保存搜索结果并转到输入成本价步骤
		m.stockInfo = stockData
		m.tempCode = stockData.Symbol
		m.addingStep = 1
		m.input = ""
		m.message = ""
	case 1: // 输入成本价
		if m.input == "" {
			m.message = m.getText("costRequired")
			return m, nil
		}
		if _, err := strconv.ParseFloat(m.input, 64); err != nil {
			m.message = m.getText("invalidPrice")
			m.input = ""
			return m, nil
		}
		m.tempCost = m.input
		m.addingStep = 2
		m.input = ""
		m.message = ""
	case 2: // 输入数量
		if m.input == "" {
			m.message = m.getText("quantityRequired")
			return m, nil
		}
		if _, err := strconv.Atoi(m.input); err != nil {
			m.message = m.getText("invalidQuantity")
			m.input = ""
			return m, nil
		}
		m.tempQuantity = m.input

		// 添加股票
		costPrice, _ := strconv.ParseFloat(m.tempCost, 64)
		quantity, _ := strconv.Atoi(m.tempQuantity)

		stock := Stock{
			Code:      m.tempCode,
			Name:      m.stockInfo.Name,
			CostPrice: costPrice,
			Quantity:  quantity,
		}

		m.portfolio.Stocks = append(m.portfolio.Stocks, stock)
		m.savePortfolio()

		// 根据来源决定跳转目标
		if m.fromSearch {
			// 从搜索结果添加，跳转到持股列表（监控）页面
			m.state = Monitoring
			m.lastUpdate = time.Now()
			m.fromSearch = false // 重置标志
			m.message = fmt.Sprintf(m.getText("addSuccess"), m.stockInfo.Name, m.tempCode)
			m.addingStep = 0
			m.input = ""
			return m, m.tickCmd() // 跳转到监控页面时启动定时器
		} else {
			// 从主菜单添加，返回主菜单
			m.state = MainMenu
			m.message = fmt.Sprintf(m.getText("addSuccess"), m.stockInfo.Name, m.tempCode)
			m.addingStep = 0
			m.input = ""
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) viewAddingStock() string {
	s := m.getText("addingTitle") + "\n\n"

	switch m.addingStep {
	case 0:
		s += m.getText("enterSearch") + m.input + "_\n"
		s += "\n" + m.getText("searchFormats") + "\n"
	case 1:
		s += fmt.Sprintf(m.getText("stockCode"), m.tempCode) + "\n"
		s += fmt.Sprintf(m.getText("stockName"), m.stockInfo.Name) + "\n"
		s += fmt.Sprintf(m.getText("currentPrice"), m.stockInfo.Price) + "\n\n"
		s += m.getText("enterCost") + m.input + "_\n"
	case 2:
		s += fmt.Sprintf(m.getText("stockCode"), m.tempCode) + "\n"
		s += fmt.Sprintf(m.getText("stockName"), m.stockInfo.Name) + "\n"
		s += fmt.Sprintf(m.getText("currentPrice"), m.stockInfo.Price) + "\n"
		s += fmt.Sprintf(m.getText("costPrice"), m.tempCost) + "\n\n"
		s += m.getText("enterQuantity") + m.input + "_\n"
	}

	s += "\n" + m.getText("returnEscOnly") + "\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) handleRemovingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		// 根据之前的状态决定返回到哪里
		if m.previousState == Monitoring {
			m.state = Monitoring
			m.lastUpdate = time.Now()
			m.message = ""
			return m, m.tickCmd()
		} else {
			m.state = MainMenu
			m.message = "" // 清除消息
			return m, nil
		}
	case "up", "k", "w":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "s":
		if m.cursor < len(m.portfolio.Stocks)-1 {
			m.cursor++
		}
	case "enter", " ":
		if len(m.portfolio.Stocks) > 0 {
			removedStock := m.portfolio.Stocks[m.cursor]
			m.portfolio.Stocks = append(m.portfolio.Stocks[:m.cursor], m.portfolio.Stocks[m.cursor+1:]...)
			m.savePortfolio()
			// 根据之前的状态决定返回到哪里
			if m.previousState == Monitoring {
				m.state = Monitoring
				m.lastUpdate = time.Now()
				m.message = fmt.Sprintf(m.getText("removeSuccess"), removedStock.Name, removedStock.Code)
				return m, m.tickCmd()
			} else {
				m.state = MainMenu
				m.message = fmt.Sprintf(m.getText("removeSuccess"), removedStock.Name, removedStock.Code)
			}
		}
	}
	return m, nil
}

func (m *Model) viewRemovingStock() string {
	s := m.getText("removeTitle") + "\n\n"

	if len(m.portfolio.Stocks) == 0 {
		s += m.getText("emptyPortfolio") + "\n\n" + m.getText("returnToMenuShort") + "\n"
		return s
	}

	s += m.getText("selectToRemove") + "\n\n"
	for i, stock := range m.portfolio.Stocks {
		prefix := "  "
		if i == m.cursor {
			prefix = "► "
		}
		s += fmt.Sprintf("%s%d. %s (%s)\n", prefix, i+1, stock.Name, stock.Code)
	}

	s += "\n" + m.getText("navHelp") + "\n"
	return s
}

func (m *Model) handleMonitoring(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "m":
		m.state = MainMenu
		m.message = "" // 清除消息
		return m, nil
	case "e":
		// 修改股票功能
		if len(m.portfolio.Stocks) == 0 {
			m.message = m.getText("emptyCannotEdit")
			return m, nil
		}
		m.previousState = m.state // 记录当前状态
		m.state = EditingStock
		m.editingStep = 0
		m.cursor = 0
		m.input = ""
		m.message = ""
		return m, nil
	case "d":
		// 删除股票功能
		if len(m.portfolio.Stocks) == 0 {
			m.message = m.getText("emptyPortfolio")
			return m, nil
		}
		m.previousState = m.state // 记录当前状态
		m.state = RemovingStock
		m.cursor = 0
		m.message = ""
		return m, nil
	case "a":
		// 跳转到添加股票页面
		m.logUserAction("从持股列表跳转到添加股票页面")
		m.previousState = m.state // 记录当前状态
		m.state = AddingStock
		m.addingStep = 0
		m.tempCode = ""
		m.tempCost = ""
		m.tempQuantity = ""
		m.stockInfo = nil
		m.input = ""
		m.message = ""
		m.fromSearch = true // 设置标志，表示从持股列表进入，完成后应该回到监控页面
		return m, nil
	case "up", "k", "w":
		if m.portfolioCursor > 0 {
			m.portfolioCursor--
		}
		return m, nil
	case "down", "j", "s":
		if m.portfolioCursor < len(m.portfolio.Stocks)-1 {
			m.portfolioCursor++
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) viewMonitoring() string {
	s := m.getText("monitoringTitle") + "\n"
	s += fmt.Sprintf(m.getText("updateTime"), m.lastUpdate.Format("2006-01-02 15:04:05")) + "\n"
	s += "\n"

	if len(m.portfolio.Stocks) == 0 {
		s += m.getText("emptyPortfolio") + "\n\n"
		s += m.getText("addStockFirst") + "\n\n"
		s += m.getText("holdingsHelp") + "\n"
		return s
	}

	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 获取本地化的表头
	if m.language == Chinese {
		t.AppendHeader(table.Row{"", "代码", "名称", "昨收价", "现价", "成本价", "开盘", "最高", "最低", "持股数", "今日涨幅", "今日盈亏", "持仓盈亏", "盈亏率", "市值"})
	} else {
		t.AppendHeader(table.Row{"", "Code", "Name", "PrevClose", "Price", "Cost", "Open", "High", "Low", "Quantity", "Today%", "TodayP&L", "PositionP&L", "P&LRate", "Value"})
	}

	var totalMarketValue float64
	var totalCost float64
	var totalTodayProfit float64

	// 显示滚动信息
	totalStocks := len(m.portfolio.Stocks)
	maxPortfolioLines := m.config.Display.MaxLines
	if totalStocks > 0 {
		currentPos := m.portfolioCursor + 1 // 显示从1开始的位置
		if m.language == Chinese {
			s += fmt.Sprintf("📊 持股列表 (%d/%d) [↑/↓:翻页]\n", currentPos, totalStocks)
		} else {
			s += fmt.Sprintf("📊 Portfolio (%d/%d) [↑/↓:scroll]\n", currentPos, totalStocks)
		}
		s += "\n"
	}

	// 计算要显示的股票范围
	stocks := m.portfolio.Stocks
	endIndex := len(stocks) - m.portfolioScrollPos
	startIndex := endIndex - maxPortfolioLines
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(stocks) {
		endIndex = len(stocks)
	}

	// 首先计算所有股票的总计（用于汇总行）
	for i := range m.portfolio.Stocks {
		stock := &m.portfolio.Stocks[i]
		stockData := getStockPrice(stock.Code)
		if stockData != nil {
			stock.Price = stockData.Price
			stock.Change = stockData.Change
			stock.ChangePercent = stockData.ChangePercent
			stock.StartPrice = stockData.StartPrice
			stock.MaxPrice = stockData.MaxPrice
			stock.MinPrice = stockData.MinPrice
			stock.PrevClose = stockData.PrevClose
		}

		if stock.Price > 0 {
			marketValue := stock.Price * float64(stock.Quantity)
			cost := stock.CostPrice * float64(stock.Quantity)
			todayProfit := stock.Change * float64(stock.Quantity)

			totalMarketValue += marketValue
			totalCost += cost
			totalTodayProfit += todayProfit
		}
	}

	// 然后显示当前范围内的股票
	for i := startIndex; i < endIndex; i++ {
		stock := &m.portfolio.Stocks[i]

		if stock.Price > 0 {
			// 今日盈亏：今日价格变化带来的盈亏 = (现价 - 昨收价) × 持股数
			todayProfit := stock.Change * float64(stock.Quantity)
			// 持仓盈亏：基于成本价的实时盈亏状态
			positionProfit := (stock.Price - stock.CostPrice) * float64(stock.Quantity)
			profitRate := ((stock.Price - stock.CostPrice) / stock.CostPrice) * 100
			marketValue := stock.Price * float64(stock.Quantity)

			// 计算今日涨幅：应该基于昨收价，而不是开盘价
			var todayChangeStr string
			// 使用change_percent字段，这是基于昨收价计算的涨跌幅
			if stock.ChangePercent != 0 {
				todayChangeStr = m.formatProfitRateWithColorZeroLang(stock.ChangePercent)
			} else {
				todayChangeStr = "-"
			}

			// 使用多语言颜色显示函数
			todayProfitStr := m.formatProfitWithColorZeroLang(todayProfit)
			positionProfitStr := m.formatProfitWithColorZeroLang(positionProfit)
			profitRateStr := m.formatProfitRateWithColorZeroLang(profitRate)

			// 光标列 - 检查光标是否在当前可见范围内且指向此行
			cursorCol := ""
			if m.portfolioCursor >= startIndex && m.portfolioCursor < endIndex && i == m.portfolioCursor {
				cursorCol = "►"
			}

			t.AppendRow(table.Row{
				cursorCol,
				stock.Code,
				stock.Name,
				fmt.Sprintf("%.3f", stock.PrevClose), // 昨收价（无颜色）
				m.formatPriceWithColorLang(stock.Price, stock.PrevClose),      // 现价（有颜色）
				fmt.Sprintf("%.3f", stock.CostPrice),                          // 成本价（无颜色）
				m.formatPriceWithColorLang(stock.StartPrice, stock.PrevClose), // 开盘
				m.formatPriceWithColorLang(stock.MaxPrice, stock.PrevClose),   // 最高
				m.formatPriceWithColorLang(stock.MinPrice, stock.PrevClose),   // 最低
				stock.Quantity,
				todayChangeStr,
				todayProfitStr,    // 今日盈亏（基于今日价格变化）
				positionProfitStr, // 持仓盈亏（基于成本价）
				profitRateStr,
				fmt.Sprintf("%.2f", marketValue),
			})

			// 在每个股票后添加分隔线（除了显示范围内的最后一个）
			if i < endIndex-1 {
				t.AppendSeparator()
			}
		} else {
			// 如果无法获取数据，显示基本信息但标记数据不可用
			// 光标列 - 检查光标是否在当前可见范围内且指向此行
			cursorCol := ""
			if m.portfolioCursor >= startIndex && m.portfolioCursor < endIndex && i == m.portfolioCursor {
				cursorCol = "►"
			}

			t.AppendRow(table.Row{
				cursorCol,
				stock.Code,
				stock.Name,
				"-",
				"-",
				"-",
				"-",
				"-",
				fmt.Sprintf("%.3f", stock.CostPrice),
				stock.Quantity,
				"-",
				"-",
				"-",
				"-",
				"-",
			})
			// 在每个股票后添加分隔线（除了显示范围内的最后一个）
			if i < endIndex-1 {
				t.AppendSeparator()
			}
		}
	}

	totalPortfolioProfit := totalMarketValue - totalCost
	totalProfitRate := 0.0
	if totalCost > 0 {
		totalProfitRate = (totalPortfolioProfit / totalCost) * 100
	}

	t.AppendSeparator()
	t.AppendRow(table.Row{
		"",                 // 光标列
		"",                 // 代码
		m.getText("total"), // 名称 -> 总计
		"",                 // 昨收价
		"",                 // 现价
		"",                 // 成本价
		"",                 // 开盘
		"",                 // 最高
		"",                 // 最低
		"",                 // 持股数
		"",                 // 今日涨幅
		m.formatProfitWithColorLang(totalTodayProfit),     // 今日盈亏（总今日盈亏）
		m.formatProfitWithColorLang(totalPortfolioProfit), // 持仓盈亏（总持仓盈亏）
		m.formatProfitRateWithColorLang(totalProfitRate),  // 盈亏率（总盈亏率）
		fmt.Sprintf("%.2f", totalMarketValue),             // 市值（总市值）
	})

	s += t.Render() + "\n"

	// 如果可以滚动，显示滚动指示
	if totalStocks > maxPortfolioLines {
		s += strings.Repeat("-", 80) + "\n"
		if m.portfolioScrollPos > 0 {
			if m.language == Chinese {
				s += "↑ 有更新的股票 (按↓查看)\n"
			} else {
				s += "↑ Newer stocks available (press ↓)\n"
			}
		}
		if m.portfolioScrollPos < totalStocks-1 {
			if m.language == Chinese {
				s += "↓ 有更多历史股票 (按↑查看)\n"
			} else {
				s += "↓ More stocks available (press ↑)\n"
			}
		}
	}

	s += "\n" + m.getText("holdingsHelp") + "\n"

	return s
}

func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *Model) savePortfolio() {
	data, err := json.MarshalIndent(m.portfolio, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(dataFile, data, 0644)
}

// 默认配置
func getDefaultConfig() Config {
	return Config{
		System: SystemConfig{
			Language:      "en",        // 默认英文
			AutoStart:     true,        // 有数据时自动进入监控模式
			StartupModule: "portfolio", // 默认启动持股模块
			DebugMode:     false,       // 调试模式关闭
		},
		Display: DisplayConfig{
			ColorScheme:   "professional", // 专业配色方案
			DecimalPlaces: 3,              // 3位小数
			TableStyle:    "light",        // 轻量表格样式
			MaxLines:      10,             // 默认每页显示10行
		},
		Update: UpdateConfig{
			RefreshInterval: 5,    // 5秒刷新间隔
			AutoUpdate:      true, // 自动更新开启
		},
	}
}

// 加载配置文件
func loadConfig() Config {
	data, err := os.ReadFile(configFile)
	if err != nil {
		// 如果配置文件不存在，创建默认配置文件
		config := getDefaultConfig()
		saveConfig(config)
		return config
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		// 如果配置文件格式错误，使用默认配置
		return getDefaultConfig()
	}
	
	// 验证配置的合理性
	if config.Display.MaxLines <= 0 || config.Display.MaxLines > 50 {
		config.Display.MaxLines = 10 // 默认值
	}
	
	return config
}

// 保存配置文件
func saveConfig(config Config) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0644)
}

func loadPortfolio() Portfolio {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return Portfolio{Stocks: []Stock{}}
	}

	var portfolio Portfolio
	err = json.Unmarshal(data, &portfolio)
	if err != nil {
		return Portfolio{Stocks: []Stock{}}
	}
	return portfolio
}

func formatProfitWithColor(profit float64) string {
	if profit >= 0 {
		return text.FgRed.Sprintf("+%.2f", profit)
	}
	return text.FgGreen.Sprintf("%.2f", profit)
}

func formatProfitRateWithColor(rate float64) string {
	if rate >= 0 {
		return text.FgRed.Sprintf("+%.2f%%", rate)
	}
	return text.FgGreen.Sprintf("%.2f%%", rate)
}

// 支持多语言的颜色显示函数
func (m *Model) formatProfitWithColorLang(profit float64) string {
	if m.language == English {
		// 英文：绿色盈利，红色亏损
		if profit >= 0 {
			return text.FgGreen.Sprintf("+%.2f", profit)
		}
		return text.FgRed.Sprintf("%.2f", profit)
	} else {
		// 中文：红色盈利，绿色亏损
		if profit >= 0 {
			return text.FgRed.Sprintf("+%.2f", profit)
		}
		return text.FgGreen.Sprintf("%.2f", profit)
	}
}

func (m *Model) formatProfitRateWithColorLang(rate float64) string {
	if m.language == English {
		// 英文：绿色盈利，红色亏损
		if rate >= 0 {
			return text.FgGreen.Sprintf("+%.2f%%", rate)
		}
		return text.FgRed.Sprintf("%.2f%%", rate)
	} else {
		// 中文：红色盈利，绿色亏损
		if rate >= 0 {
			return text.FgRed.Sprintf("+%.2f%%", rate)
		}
		return text.FgGreen.Sprintf("%.2f%%", rate)
	}
}

func (m *Model) formatProfitWithColorZeroLang(profit float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(profit) < 0.001 {
		return fmt.Sprintf("%.2f", profit)
	}
	// 否则使用语言相关颜色逻辑
	return m.formatProfitWithColorLang(profit)
}

func (m *Model) formatProfitRateWithColorZeroLang(rate float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(rate) < 0.001 {
		return fmt.Sprintf("%.2f%%", rate)
	}
	// 否则使用语言相关颜色逻辑
	return m.formatProfitRateWithColorLang(rate)
}

func (m *Model) formatPriceWithColorLang(currentPrice, prevClose float64) string {
	if prevClose == 0 {
		// 如果昨收价为0，直接显示价格不加颜色
		return fmt.Sprintf("%.3f", currentPrice)
	}

	if currentPrice > prevClose {
		if m.language == English {
			// 英文：高于昨收价显示绿色
			return text.FgGreen.Sprintf("%.3f", currentPrice)
		} else {
			// 中文：高于昨收价显示红色
			return text.FgRed.Sprintf("%.3f", currentPrice)
		}
	} else if currentPrice < prevClose {
		if m.language == English {
			// 英文：低于昨收价显示红色
			return text.FgRed.Sprintf("%.3f", currentPrice)
		} else {
			// 中文：低于昨收价显示绿色
			return text.FgGreen.Sprintf("%.3f", currentPrice)
		}
	} else {
		// 等于昨收价显示白色（无颜色）
		return fmt.Sprintf("%.3f", currentPrice)
	}
}

// 根据数值本身判断颜色显示：0时显示白色，正数红色，负数绿色
func formatProfitWithColorZero(profit float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(profit) < 0.001 {
		return fmt.Sprintf("%.2f", profit)
	}
	// 否则使用原有颜色逻辑
	return formatProfitWithColor(profit)
}

func formatProfitRateWithColorZero(rate float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(rate) < 0.001 {
		return fmt.Sprintf("%.2f%%", rate)
	}
	// 否则使用原有颜色逻辑
	return formatProfitRateWithColor(rate)
}

// 辅助函数：计算浮点数绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// 基于昨收价比较的价格颜色显示函数
func formatPriceWithColor(currentPrice, prevClose float64) string {
	if prevClose == 0 {
		// 如果昨收价为0，直接显示价格不加颜色
		return fmt.Sprintf("%.3f", currentPrice)
	}

	if currentPrice > prevClose {
		// 高于昨收价显示红色
		return text.FgRed.Sprintf("%.3f", currentPrice)
	} else if currentPrice < prevClose {
		// 低于昨收价显示绿色
		return text.FgGreen.Sprintf("%.3f", currentPrice)
	} else {
		// 等于昨收价显示白色（无颜色）
		return fmt.Sprintf("%.3f", currentPrice)
	}
}

func getStockInfo(symbol string) *StockData {
	// 如果输入是中文，尝试通过API搜索
	if containsChineseChars(symbol) {
		return searchChineseStock(symbol)
	}
	return getStockPrice(symbol)
}

// 检查字符串是否包含中文字符
func containsChineseChars(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// 通过API搜索中文股票名称
func searchChineseStock(chineseName string) *StockData {
	chineseName = strings.TrimSpace(chineseName)
	debugPrint("[调试] 开始搜索中文股票: %s\n", chineseName)

	// 策略1: 使用腾讯搜索API
	result := searchStockByTencentAPI(chineseName)
	if result != nil && result.Price > 0 {
		debugPrint("[调试] 腾讯搜索API成功找到: %s (%s)\n", result.Name, result.Symbol)
		return result
	}

	// 策略2: 尝试新浪财经搜索API
	result = searchStockBySinaAPI(chineseName)
	if result != nil && result.Price > 0 {
		debugPrint("[调试] 新浪搜索API成功找到: %s (%s)\n", result.Name, result.Symbol)
		return result
	}

	// 策略3: 尝试更多的搜索关键词变形
	result = tryAdvancedSearch(chineseName)
	if result != nil && result.Price > 0 {
		debugPrint("[调试] 高级搜索成功找到: %s (%s)\n", result.Name, result.Symbol)
		return result
	}

	// 所有搜索策略都失败
	debugPrint("[调试] 所有搜索策略都失败，未找到股票数据\n")
	return nil
}

// 使用腾讯搜索API查找股票
func searchStockByTencentAPI(keyword string) *StockData {
	debugPrint("[调试] 使用腾讯搜索API查找: %s\n", keyword)

	// 腾讯股票搜索API URL - 使用更完整的搜索接口
	url := fmt.Sprintf("https://smartbox.gtimg.cn/s3/?q=%s&t=gp", keyword)
	debugPrint("[调试] 腾讯搜索请求URL: %s\n", url)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		debugPrint("[错误] 腾讯搜索API创建请求失败: %v\n", err)
		return nil
	}

	// 添加必要的请求头，提高成功率
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://stockapp.finance.qq.com/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		debugPrint("[错误] 腾讯搜索API HTTP请求失败: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		debugPrint("[错误] 腾讯搜索API返回非200状态码: %d\n", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] 腾讯搜索API读取响应失败: %v\n", err)
		return nil
	}

	content, err := gbkToUtf8(body)
	if err != nil {
		debugPrint("[错误] 腾讯搜索API编码转换失败: %v\n", err)
		content = string(body)
	}
	debugPrint("[调试] 腾讯搜索API响应: %s\n", content[:min(300, len(content))])

	// 解析搜索结果
	return parseSearchResults(content, keyword)
}

// 解析腾讯搜索结果
func parseSearchResults(content, keyword string) *StockData {
	debugPrint("[调试] 开始解析搜索结果\n")

	// 尝试解析新的腾讯格式 (v_hint=)
	result := parseTencentHintFormat(content, keyword)
	if result != nil {
		return result
	}

	// 尝试解析JSON格式的响应
	result = parseJSONSearchResults(content, keyword)
	if result != nil {
		return result
	}

	// 如果JSON解析失败，尝试解析旧格式
	return parseLegacySearchResults(content, keyword)
}

// 解析腾讯Hint格式的搜索结果
func parseTencentHintFormat(content, keyword string) *StockData {
	// 格式: v_hint="sz~000880~潍柴重机~wczj~GP-A"
	debugPrint("[调试] 尝试解析腾讯Hint格式\n")

	// 查找v_hint=
	if !strings.Contains(content, "v_hint=") {
		return nil
	}

	// 提取引号内的内容
	startPos := strings.Index(content, "v_hint=\"")
	if startPos == -1 {
		return nil
	}
	startPos += len("v_hint=\"")

	endPos := strings.Index(content[startPos:], "\"")
	if endPos == -1 {
		return nil
	}

	data := content[startPos : startPos+endPos]
	debugPrint("[调试] 提取的数据: %s\n", data)

	// 按^分割多个结果，取第一个
	results := strings.Split(data, "^")
	if len(results) == 0 {
		debugPrint("[调试] 未找到搜索结果\n")
		return nil
	}

	// 处理第一个结果
	firstResult := results[0]
	fields := strings.Split(firstResult, "~")
	if len(fields) < 3 {
		debugPrint("[调试] 字段数量不足: %d\n", len(fields))
		return nil
	}

	market := fields[0] // sz, sh, hk
	code := fields[1]   // 000880
	name := fields[2]   // 潍柴重机（可能是Unicode编码）

	// 尝试解码Unicode字符串
	decodedName, err := strconv.Unquote(`"` + name + `"`)
	if err == nil {
		name = decodedName
	}

	debugPrint("[调试] 解析结果 - 市场: %s, 代码: %s, 名称: %s\n", market, code, name)

	// 对于搜索结果，直接返回第一个匹配项（因为用户输入的关键词已经被API处理过了）
	if true {
		// 转换为标准格式
		standardCode := strings.ToUpper(market) + code
		debugPrint("[调试] 腾讯Hint格式找到匹配股票: %s (%s)\n", name, standardCode)

		// 获取详细信息
		stockData := getStockPrice(standardCode)
		if stockData != nil && stockData.Price > 0 {
			stockData.Symbol = standardCode
			stockData.Name = name
			return stockData
		}
	}

	return nil
}

// 解析JSON格式的搜索结果
func parseJSONSearchResults(content, keyword string) *StockData {
	// 尝试解析为JSON
	var searchResult map[string]interface{}
	if err := json.Unmarshal([]byte(content), &searchResult); err != nil {
		debugPrint("[调试] JSON解析失败: %v\n", err)
		return nil
	}

	// 查找数据字段
	data, ok := searchResult["data"]
	if !ok {
		debugPrint("[调试] 找不到data字段\n")
		return nil
	}

	dataArray, ok := data.([]interface{})
	if !ok {
		debugPrint("[调试] data不是数组格式\n")
		return nil
	}

	for _, item := range dataArray {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// 提取股票信息
		code, _ := itemMap["code"].(string)
		name, _ := itemMap["name"].(string)

		if code == "" || name == "" {
			continue
		}

		// 检查名称是否匹配关键词
		if strings.Contains(name, keyword) {
			debugPrint("[调试] JSON格式找到匹配股票: %s (%s)\n", name, code)

			// 转换为标准格式
			standardCode := convertJSONCodeToStandard(code)

			// 获取详细信息
			stockData := getStockPrice(standardCode)
			if stockData != nil && stockData.Price > 0 {
				stockData.Symbol = standardCode
				stockData.Name = name
				return stockData
			}
		}
	}

	return nil
}

// 解析旧格式的搜索结果
func parseLegacySearchResults(content, keyword string) *StockData {
	debugPrint("[调试] 使用旧格式解析\n")
	// 腾讯搜索结果格式分析
	// 格式类似: v_s_关键词="sz002415~海康威视~002415~7.450~-0.160~-2.105~15270~7705~7565~7.610"
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		if !strings.Contains(line, "~") {
			continue
		}

		// 找到符号="的位置
		startPos := strings.Index(line, "\"")
		endPos := strings.LastIndex(line, "\"")
		if startPos == -1 || endPos == -1 || startPos >= endPos {
			continue
		}

		// 提取数据部分
		data := line[startPos+1 : endPos]
		fields := strings.Split(data, "~")

		if len(fields) < 4 {
			continue
		}

		// 解析字段
		code := fields[0]
		name := fields[1]
		shortCode := fields[2]

		// 检查名称是否匹配关键词
		if strings.Contains(name, keyword) {
			debugPrint("[调试] 旧格式找到匹配股票: %s (%s)\n", name, code)

			// 转换为标准格式
			standardCode := convertToStandardCode(code, shortCode)

			// 获取详细信息
			stockData := getStockPrice(standardCode)
			if stockData != nil && stockData.Price > 0 {
				stockData.Symbol = standardCode
				stockData.Name = name
				return stockData
			}
		}
	}

	return nil
}

// 转换JSON格式的股票代码为标准格式
func convertJSONCodeToStandard(code string) string {
	code = strings.TrimSpace(code)

	// 如果已经是标准格式，直接返回
	if strings.HasPrefix(code, "SH") || strings.HasPrefix(code, "SZ") || strings.HasPrefix(code, "HK") {
		return code
	}

	// 根据数字开头判断市场
	if len(code) == 6 {
		if strings.HasPrefix(code, "6") {
			return "SH" + code
		} else if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
			return "SZ" + code
		}
	}

	return code
}

// 将腾讯的股票代码转换为标准格式
func convertToStandardCode(code, shortCode string) string {
	code = strings.ToLower(strings.TrimSpace(code))

	if strings.HasPrefix(code, "sh") {
		return "SH" + shortCode
	} else if strings.HasPrefix(code, "sz") {
		return "SZ" + shortCode
	} else if strings.HasPrefix(code, "hk") {
		return "HK" + shortCode
	}

	// 如果无法识别，返回原始代码
	return code
}

// 使用新浪财经搜索API查找股票
func searchStockBySinaAPI(keyword string) *StockData {
	debugPrint("[调试] 使用新浪财经搜索API查找: %s\n", keyword)

	// 新浪财经搜索API URL
	url := fmt.Sprintf("https://suggest3.sinajs.cn/suggest/type=11,12,13,14,15&key=%s", keyword)
	debugPrint("[调试] 新浪财经请求URL: %s\n", url)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		debugPrint("[错误] 新浪财经搜索API HTTP请求失败: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] 新浪财经搜索API读取响应失败: %v\n", err)
		return nil
	}

	content := string(body)
	debugPrint("[调试] 新浪财经搜索API响应: %s\n", content[:min(200, len(content))])

	// 解析新浪搜索结果
	return parseSinaSearchResults(content, keyword)
}

// 解析新浪搜索结果
func parseSinaSearchResults(content, keyword string) *StockData {
	// 新浪返回格式类似: var suggestvalue="sz000858,五粮液;sh600519,贵州茅台;";
	lines := strings.Split(content, ";")

	for _, line := range lines {
		if !strings.Contains(line, ",") {
			continue
		}

		// 提取股票信息
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}

		code := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])

		// 清理代码和名称中的特殊字符
		code = strings.Trim(code, "\"'")
		name = strings.Trim(name, "\"'")

		if code == "" || name == "" {
			continue
		}

		// 检查名称是否匹配关键词
		if strings.Contains(name, keyword) {
			debugPrint("[调试] 新浪搜索找到匹配股票: %s (%s)\n", name, code)

			// 转换为标准格式
			standardCode := convertSinaCodeToStandard(code)

			// 获取详细信息
			stockData := getStockPrice(standardCode)
			if stockData != nil && stockData.Price > 0 {
				stockData.Symbol = standardCode
				stockData.Name = name
				return stockData
			}
		}
	}

	return nil
}

// 转换新浪的股票代码为标准格式
func convertSinaCodeToStandard(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))

	// 如果已经是标准格式，直接返回
	if strings.HasPrefix(strings.ToUpper(code), "SH") || strings.HasPrefix(strings.ToUpper(code), "SZ") {
		return strings.ToUpper(code)
	}

	if strings.HasPrefix(code, "sh") {
		return "SH" + strings.TrimPrefix(code, "sh")
	} else if strings.HasPrefix(code, "sz") {
		return "SZ" + strings.TrimPrefix(code, "sz")
	} else if strings.HasPrefix(code, "hk") {
		return "HK" + strings.TrimPrefix(code, "hk")
	}

	// 如果是6位数字，根据开头判断市场
	if len(code) == 6 {
		if strings.HasPrefix(code, "6") {
			return "SH" + code
		} else if strings.HasPrefix(code, "0") || strings.HasPrefix(code, "3") {
			return "SZ" + code
		}
	}

	return strings.ToUpper(code)
}

// 高级搜索策略：尝试多种关键词变形
func tryAdvancedSearch(chineseName string) *StockData {
	// 生成搜索关键词变形
	keywords := generateSearchKeywords(chineseName)

	for _, keyword := range keywords {
		if keyword == chineseName {
			continue // 跳过原始关键词，避免重复搜索
		}

		debugPrint("[调试] 尝试搜索关键词变形: %s\n", keyword)
		result := searchStockByTencentAPI(keyword)
		if result != nil && result.Price > 0 {
			return result
		}
	}

	return nil
}

// 生成搜索关键词变形
func generateSearchKeywords(name string) []string {
	var keywords []string

	// 原始关键词
	keywords = append(keywords, name)

	// 如果名称包含“股份”、“集团”等后缀，尝试去掉
	suffixes := []string{"股份", "集团", "公司", "有限公司", "科技", "实业"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			shortName := strings.TrimSuffix(name, suffix)
			if len(shortName) > 1 {
				keywords = append(keywords, shortName)
			}
		}
	}

	// 如果名称包含“中国”、“上海”等前缀，尝试去掉
	prefixes := []string{"中国", "上海", "北京", "广东", "深圳", "天津"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix)+1 {
			shortName := strings.TrimPrefix(name, prefix)
			if len(shortName) > 1 {
				keywords = append(keywords, shortName)
			}
		}
	}

	// 如果名称较长，尝试取前几个字符作为关键词
	if len([]rune(name)) > 4 {
		runes := []rune(name)
		// 取前3个字符
		if len(runes) >= 3 {
			keywords = append(keywords, string(runes[:3]))
		}
		// 取前4个字符
		if len(runes) >= 4 {
			keywords = append(keywords, string(runes[:4]))
		}
	}

	return keywords
}

func getStockPrice(symbol string) *StockData {
	if isChinaStock(symbol) {
		data := tryTencentAPI(symbol)
		if data.Price > 0 {
			return data
		}
		debugPrint("[调试] 腾讯API失败，尝试其他API\n")
	}

	data := tryFinnhubAPI(symbol)
	if data.Price > 0 {
		return data
	}

	debugPrint("[调试] 所有API都失败，未找到股票数据\n")
	return nil
}

func isChinaStock(symbol string) bool {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	return strings.HasPrefix(symbol, "SH") || strings.HasPrefix(symbol, "SZ") ||
		(len(symbol) == 6 && (strings.HasPrefix(symbol, "0") || strings.HasPrefix(symbol, "3") || strings.HasPrefix(symbol, "6")))
}

func tryTencentAPI(symbol string) *StockData {
	tencentSymbol := convertStockSymbolForTencent(symbol)
	debugPrint("[调试] 腾讯API - 原始代码: %s -> 转换后: %s\n", symbol, tencentSymbol)

	url := fmt.Sprintf("https://qt.gtimg.cn/q=%s", tencentSymbol)
	debugPrint("[调试] 腾讯请求URL: %s\n", url)

	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		debugPrint("[错误] 腾讯价格API创建请求失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	// 添加必要的请求头，与搜索API保持一致
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://stockapp.finance.qq.com/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		debugPrint("[错误] 腾讯API HTTP请求失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] 腾讯API读取响应失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	content, err := gbkToUtf8(body)
	if err != nil {
		debugPrint("[错误] 腾讯API编码转换失败: %v\n", err)
		content = string(body)
	}
	debugPrint("[调试] 腾讯API响应: %s\n", content[:min(100, len(content))])

	if !strings.Contains(content, "~") {
		debugPrint("[调试] 腾讯API响应格式错误\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	fields := strings.Split(content, "~")
	if len(fields) < 5 {
		debugPrint("[调试] 腾讯API数据字段不足\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	stockName := fields[1]

	price, err := strconv.ParseFloat(fields[3], 64)
	if err != nil || price <= 0 {
		debugPrint("[调试] 腾讯API价格解析失败: %s\n", fields[3])
		return &StockData{Symbol: symbol, Price: 0}
	}

	previousClose, err := strconv.ParseFloat(fields[4], 64)
	if err != nil || previousClose <= 0 {
		debugPrint("[调试] 腾讯API昨收价解析失败: %s\n", fields[4])
		return &StockData{Symbol: symbol, Price: 0}
	}

	// 解析开盘价、最高价、最低价、换手率、成交量
	var openPrice, maxPrice, minPrice, turnoverRate float64
	var volume int64

	// 腾讯API字段位置：fields[5]=开盘价, fields[33]=最高价, fields[34]=最低价, fields[38]=换手率, fields[36]=成交量
	if len(fields) > 5 {
		openPrice, _ = strconv.ParseFloat(fields[5], 64)
	}
	if len(fields) > 33 {
		maxPrice, _ = strconv.ParseFloat(fields[33], 64)
	}
	if len(fields) > 34 {
		minPrice, _ = strconv.ParseFloat(fields[34], 64)
	}
	if len(fields) > 38 {
		turnoverRate, _ = strconv.ParseFloat(fields[38], 64)
	}
	if len(fields) > 36 {
		volume, _ = strconv.ParseInt(fields[36], 10, 64)
	}

	change := price - previousClose
	changePercent := (change / previousClose) * 100

	debugPrint("[调试] 腾讯API获取成功 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%), 开: %.2f, 高: %.2f, 低: %.2f, 换手: %.2f%%, 量: %d\n",
		stockName, price, change, changePercent, openPrice, maxPrice, minPrice, turnoverRate, volume)

	return &StockData{
		Symbol:        symbol,
		Name:          stockName,
		Price:         price,
		Change:        change,
		ChangePercent: changePercent,
		StartPrice:    openPrice,
		MaxPrice:      maxPrice,
		MinPrice:      minPrice,
		PrevClose:     previousClose,
		TurnoverRate:  turnoverRate,
		Volume:        volume,
	}
}

func convertStockSymbolForTencent(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	if strings.HasPrefix(symbol, "SH") {
		return "sh" + strings.TrimPrefix(symbol, "SH")
	} else if strings.HasPrefix(symbol, "SZ") {
		return "sz" + strings.TrimPrefix(symbol, "SZ")
	}

	if len(symbol) == 6 && strings.HasPrefix(symbol, "6") {
		return "sh" + symbol
	} else if len(symbol) == 6 && (strings.HasPrefix(symbol, "0") || strings.HasPrefix(symbol, "3")) {
		return "sz" + symbol
	}

	return symbol
}

func tryFinnhubAPI(symbol string) *StockData {
	convertedSymbol := convertStockSymbolForFinnhub(symbol)
	debugPrint("[调试] Finnhub - 原始代码: %s -> 转换后: %s\n", symbol, convertedSymbol)

	stockName := getFinnhubStockName(convertedSymbol)

	url := fmt.Sprintf("https://finnhub.io/api/v1/quote?symbol=%s&token=demo", convertedSymbol)
	debugPrint("[调试] Finnhub请求URL: %s\n", url)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		debugPrint("[错误] Finnhub HTTP请求失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		debugPrint("[错误] Finnhub读取响应失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		debugPrint("[错误] Finnhub JSON解析失败: %v\n", err)
		return &StockData{Symbol: symbol, Price: 0}
	}

	current, currentOk := result["c"].(float64)
	previous, prevOk := result["pc"].(float64)

	if !currentOk || !prevOk || current <= 0 {
		debugPrint("[调试] Finnhub数据无效或为空\n")
		return &StockData{Symbol: symbol, Price: 0}
	}

	change := current - previous
	changePercent := (change / previous) * 100

	debugPrint("[调试] Finnhub获取成功 - 名称: %s, 价格: %.2f, 涨跌: %.2f (%.2f%%)\n", stockName, current, change, changePercent)

	return &StockData{
		Symbol:        symbol,
		Name:          stockName,
		Price:         current,
		Change:        change,
		ChangePercent: changePercent,
		PrevClose:     previous,
		TurnoverRate:  0,
		Volume:        0,
	}
}

func convertStockSymbolForFinnhub(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	if strings.HasPrefix(symbol, "SH") {
		return strings.TrimPrefix(symbol, "SH") + ".SS"
	} else if strings.HasPrefix(symbol, "SZ") {
		return strings.TrimPrefix(symbol, "SZ") + ".SZ"
	} else if strings.HasPrefix(symbol, "HK") {
		return strings.TrimPrefix(symbol, "HK") + ".HK"
	}

	if len(symbol) == 6 && strings.HasPrefix(symbol, "6") {
		return symbol + ".SS"
	} else if len(symbol) == 6 && (strings.HasPrefix(symbol, "0") || strings.HasPrefix(symbol, "3")) {
		return symbol + ".SZ"
	}

	return symbol
}

func getFinnhubStockName(symbol string) string {
	url := fmt.Sprintf("https://finnhub.io/api/v1/stock/profile2?symbol=%s&token=demo", symbol)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		debugPrint("[调试] 无法获取股票名称\n")
		return symbol
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return symbol
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return symbol
	}

	if name, ok := result["name"].(string); ok && name != "" {
		return name
	}

	return symbol
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Global variable to hold model reference for debug logging
var globalModel *Model

func debugPrint(format string, args ...any) {
	if globalModel != nil && globalModel.debugMode {
		timestamp := time.Now().Format("15:04:05")
		logMsg := fmt.Sprintf("[%s] %s", timestamp, fmt.Sprintf(format, args...))
		globalModel.addDebugLog(logMsg)
	}
}

func (m *Model) debugPrint(format string, args ...any) {
	if m.debugMode {
		timestamp := time.Now().Format("15:04:05")
		logMsg := fmt.Sprintf("[%s] %s", timestamp, fmt.Sprintf(format, args...))
		m.addDebugLog(logMsg)
	}
}

func (m *Model) addDebugLog(msg string) {
	// 无限制存储所有调试日志
	m.debugLogs = append(m.debugLogs, msg)

	// 关键修复：当新日志添加时，如果用户不在底部查看，需要调整滚动位置
	// 以保持用户当前查看的内容不发生错位
	if m.debugScrollPos > 0 {
		// 用户在查看历史日志，滚动位置需要增加1以保持查看的内容不变
		m.debugScrollPos++
	}
	// 如果 debugScrollPos == 0，用户在底部，自动跟随最新日志（无需调整）
}

// debug日志滚动控制方法
func (m *Model) scrollDebugUp() {
	maxScroll := len(m.debugLogs) - 1
	if m.debugScrollPos < maxScroll {
		m.debugScrollPos++
	}
}

func (m *Model) scrollDebugDown() {
	if m.debugScrollPos > 0 {
		m.debugScrollPos--
	}
}

func (m *Model) scrollDebugToTop() {
	if len(m.debugLogs) > 0 {
		m.debugScrollPos = len(m.debugLogs) - 1
	}
}

func (m *Model) scrollDebugToBottom() {
	m.debugScrollPos = 0
}

// ========== 持股列表滚动控制方法 ==========

func (m *Model) scrollPortfolioUp() {
	// 向上翻页：显示更早的股票，光标也向上移动
	if m.portfolioCursor > 0 {
		m.portfolioCursor--
	}
	// 确保光标在可见范围内，如果需要则调整滚动位置
	maxPortfolioLines := m.config.Display.MaxLines
	endIndex := len(m.portfolio.Stocks) - m.portfolioScrollPos
	startIndex := endIndex - maxPortfolioLines
	if startIndex < 0 {
		startIndex = 0
	}
	
	// 如果光标超出可见范围的上边界，调整滚动位置
	if m.portfolioCursor < startIndex {
		m.portfolioScrollPos = len(m.portfolio.Stocks) - m.portfolioCursor - maxPortfolioLines
		if m.portfolioScrollPos < 0 {
			m.portfolioScrollPos = 0
		}
	}
}

func (m *Model) scrollPortfolioDown() {
	// 向下翻页：显示更新的股票，光标也向下移动
	if m.portfolioCursor < len(m.portfolio.Stocks)-1 {
		m.portfolioCursor++
	}
	// 确保光标在可见范围内，如果需要则调整滚动位置
	maxPortfolioLines := m.config.Display.MaxLines
	endIndex := len(m.portfolio.Stocks) - m.portfolioScrollPos
	startIndex := endIndex - maxPortfolioLines
	if startIndex < 0 {
		startIndex = 0
	}
	
	// 如果光标超出可见范围的下边界，调整滚动位置
	if m.portfolioCursor >= endIndex {
		m.portfolioScrollPos = len(m.portfolio.Stocks) - m.portfolioCursor - 1
		if m.portfolioScrollPos < 0 {
			m.portfolioScrollPos = 0
		}
	}
}

func (m *Model) scrollPortfolioToTop() {
	if len(m.portfolio.Stocks) > 0 {
		m.portfolioScrollPos = len(m.portfolio.Stocks) - 1
		m.portfolioCursor = 0 // 指向最早的股票
	}
}

func (m *Model) scrollPortfolioToBottom() {
	m.portfolioScrollPos = 0
	if len(m.portfolio.Stocks) > 0 {
		m.portfolioCursor = len(m.portfolio.Stocks) - 1 // 指向最新的股票
	}
}

// ========== 自选列表滚动控制方法 ==========

func (m *Model) scrollWatchlistUp() {
	// 向上翻页：显示更早的股票，光标也向上移动
	if m.watchlistCursor > 0 {
		m.watchlistCursor--
	}
	// 确保光标在可见范围内，如果需要则调整滚动位置
	maxWatchlistLines := m.config.Display.MaxLines
	endIndex := len(m.watchlist.Stocks) - m.watchlistScrollPos
	startIndex := endIndex - maxWatchlistLines
	if startIndex < 0 {
		startIndex = 0
	}
	
	// 如果光标超出可见范围的上边界，调整滚动位置
	if m.watchlistCursor < startIndex {
		m.watchlistScrollPos = len(m.watchlist.Stocks) - m.watchlistCursor - maxWatchlistLines
		if m.watchlistScrollPos < 0 {
			m.watchlistScrollPos = 0
		}
	}
}

func (m *Model) scrollWatchlistDown() {
	// 向下翻页：显示更新的股票，光标也向下移动
	if m.watchlistCursor < len(m.watchlist.Stocks)-1 {
		m.watchlistCursor++
	}
	// 确保光标在可见范围内，如果需要则调整滚动位置
	maxWatchlistLines := m.config.Display.MaxLines
	endIndex := len(m.watchlist.Stocks) - m.watchlistScrollPos
	startIndex := endIndex - maxWatchlistLines
	if startIndex < 0 {
		startIndex = 0
	}
	
	// 如果光标超出可见范围的下边界，调整滚动位置
	if m.watchlistCursor >= endIndex {
		m.watchlistScrollPos = len(m.watchlist.Stocks) - m.watchlistCursor - 1
		if m.watchlistScrollPos < 0 {
			m.watchlistScrollPos = 0
		}
	}
}

func (m *Model) scrollWatchlistToTop() {
	if len(m.watchlist.Stocks) > 0 {
		m.watchlistScrollPos = len(m.watchlist.Stocks) - 1
		m.watchlistCursor = 0 // 指向最早的股票
	}
}

func (m *Model) scrollWatchlistToBottom() {
	m.watchlistScrollPos = 0
	if len(m.watchlist.Stocks) > 0 {
		m.watchlistCursor = len(m.watchlist.Stocks) - 1 // 指向最新的股票
	}
}

func (m *Model) logUserAction(action string) {
	if m.debugMode {
		timestamp := time.Now().Format("15:04:05")
		logMsg := fmt.Sprintf("[%s] 用户操作: %s", timestamp, action)
		m.addDebugLog(logMsg)
	}
}

func (m *Model) renderDebugPanel() string {
	if !m.debugMode {
		return ""
	}

	// 显示最多8条完整日志，支持滚动查看
	maxDebugLines := 8

	// 只有在有日志时才显示debug面板
	if len(m.debugLogs) == 0 {
		return "\n🔧 Debug Mode: ON (暂无日志)"
	}

	s := "\n" + strings.Repeat("=", 80) + "\n"

	// 显示滚动信息和快捷键提示
	totalLogs := len(m.debugLogs)
	currentPos := totalLogs - m.debugScrollPos

	if m.language == Chinese {
		s += fmt.Sprintf("🔧 调试日志 (%d/%d) [PageUp/PageDown:翻页 Home/End:首尾]\n", currentPos, totalLogs)
	} else {
		s += fmt.Sprintf("🔧 Debug Logs (%d/%d) [PageUp/PageDown:scroll Home/End:top/bottom]\n", currentPos, totalLogs)
	}
	s += strings.Repeat("-", 80) + "\n"

	// 根据滚动位置计算要显示的日志范围
	logs := m.debugLogs
	endIndex := len(logs) - m.debugScrollPos
	startIndex := endIndex - maxDebugLines
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(logs) {
		endIndex = len(logs)
	}

	// 显示当前窗口内的日志
	for i := startIndex; i < endIndex; i++ {
		// 显示完整的日志内容，不截断
		prefix := ""
		if i == endIndex-1 && m.debugScrollPos == 0 {
			prefix = "→ " // 标记最新日志
		}
		s += prefix + logs[i] + "\n"
	}

	// 如果可以滚动，显示滚动指示
	if totalLogs > maxDebugLines {
		s += strings.Repeat("-", 80) + "\n"
		if m.debugScrollPos > 0 {
			if m.language == Chinese {
				s += "↑ 有更新的日志 (按PageDown查看 或 End键跳到最新)\n"
			} else {
				s += "↑ Newer logs available (press PageDown or End to jump to latest)\n"
			}
		}
		if m.debugScrollPos < totalLogs-1 {
			if m.language == Chinese {
				s += "↓ 有更多历史日志 (按PageUp查看 或 Home键跳到最早)\n"
			} else {
				s += "↓ More history logs (press PageUp or Home to jump to oldest)\n"
			}
		}
	}

	s += strings.Repeat("=", 80)

	return s
}

func (m *Model) handleEditingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		// 根据之前的状态决定返回到哪里
		if m.previousState == Monitoring {
			m.state = Monitoring
			m.lastUpdate = time.Now()
			m.message = ""
			return m, m.tickCmd()
		} else {
			m.state = MainMenu
			m.message = ""
			return m, nil
		}
	case "up", "k", "w":
		if m.editingStep == 0 && m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "s":
		if m.editingStep == 0 && m.cursor < len(m.portfolio.Stocks)-1 {
			m.cursor++
		}
	case "enter", " ":
		return m.processEditingStep()
	case "backspace":
		if len(m.input) > 0 {
			// 正确处理多字节字符（如中文）的删除
			runes := []rune(m.input)
			if len(runes) > 0 {
				m.input = string(runes[:len(runes)-1])
			}
		}
	default:
		// 改进的输入处理：支持多字节字符（如中文）
		str := msg.String()
		if len(str) > 0 && str != "\n" && str != "\r" && !isControlKey(str) {
			m.input += str
		}
	}
	return m, nil
}

func (m *Model) processEditingStep() (tea.Model, tea.Cmd) {
	switch m.editingStep {
	case 0: // 选择股票
		if len(m.portfolio.Stocks) > 0 {
			m.selectedStockIndex = m.cursor
			m.editingStep = 1
			m.input = fmt.Sprintf("%.3f", m.portfolio.Stocks[m.selectedStockIndex].CostPrice)
		}
	case 1: // 修改成本价
		if m.input == "" {
			m.message = m.getText("costRequired")
			return m, nil
		}
		if newCost, err := strconv.ParseFloat(m.input, 64); err != nil {
			m.message = m.getText("invalidPrice")
			m.input = ""
			return m, nil
		} else {
			m.portfolio.Stocks[m.selectedStockIndex].CostPrice = newCost
			m.editingStep = 2
			m.input = fmt.Sprintf("%d", m.portfolio.Stocks[m.selectedStockIndex].Quantity)
			m.message = ""
		}
	case 2: // 修改数量
		if m.input == "" {
			m.message = m.getText("quantityRequired")
			return m, nil
		}
		if newQuantity, err := strconv.Atoi(m.input); err != nil {
			m.message = m.getText("invalidQuantity")
			m.input = ""
			return m, nil
		} else {
			m.portfolio.Stocks[m.selectedStockIndex].Quantity = newQuantity
			m.savePortfolio()

			stockName := m.portfolio.Stocks[m.selectedStockIndex].Name
			// 根据之前的状态决定返回到哪里
			if m.previousState == Monitoring {
				m.state = Monitoring
				m.lastUpdate = time.Now()
				m.message = fmt.Sprintf(m.getText("editSuccess"), stockName)
				m.editingStep = 0
				m.input = ""
				return m, m.tickCmd()
			} else {
				m.state = MainMenu
				m.message = fmt.Sprintf(m.getText("editSuccess"), stockName)
				m.editingStep = 0
				m.input = ""
			}
		}
	}
	return m, nil
}

func (m *Model) viewEditingStock() string {
	s := m.getText("editTitle") + "\n\n"

	switch m.editingStep {
	case 0:
		s += m.getText("selectToEdit") + "\n\n"
		for i, stock := range m.portfolio.Stocks {
			prefix := "  "
			if i == m.cursor {
				prefix = "► "
			}
			// 根据语言显示不同的格式
			if m.language == Chinese {
				s += fmt.Sprintf("%s%d. %s (%s) - 成本价: %.3f, 数量: %d\n",
					prefix, i+1, stock.Name, stock.Code, stock.CostPrice, stock.Quantity)
			} else {
				s += fmt.Sprintf("%s%d. %s (%s) - Cost: %.3f, Quantity: %d\n",
					prefix, i+1, stock.Name, stock.Code, stock.CostPrice, stock.Quantity)
			}
		}
		s += "\n" + m.getText("navHelp") + "\n"
	case 1:
		stock := m.portfolio.Stocks[m.selectedStockIndex]
		if m.language == Chinese {
			s += fmt.Sprintf("股票: %s (%s)\n", stock.Name, stock.Code)
		} else {
			s += fmt.Sprintf("Stock: %s (%s)\n", stock.Name, stock.Code)
		}
		s += fmt.Sprintf(m.getText("currentCost"), stock.CostPrice) + "\n\n"
		s += m.getText("enterNewCost") + m.input + "_\n"
		s += "\n" + m.getText("returnToMenuShort") + "\n"
	case 2:
		stock := m.portfolio.Stocks[m.selectedStockIndex]
		if m.language == Chinese {
			s += fmt.Sprintf("股票: %s (%s)\n", stock.Name, stock.Code)
		} else {
			s += fmt.Sprintf("Stock: %s (%s)\n", stock.Name, stock.Code)
		}
		s += fmt.Sprintf(m.getText("newCost"), stock.CostPrice) + "\n"
		s += fmt.Sprintf(m.getText("currentQuantity"), stock.Quantity) + "\n\n"
		s += m.getText("enterNewQuantity") + m.input + "_\n"
		s += "\n" + m.getText("returnToMenuShort") + "\n"
	}

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) handleSearchingStock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.searchFromWatchlist {
			m.state = WatchlistViewing
			m.searchFromWatchlist = false
		} else {
			m.state = MainMenu
		}
		m.message = ""
		return m, nil
	case "enter":
		if m.searchInput == "" {
			m.message = m.getText("enterSearch")[:len(m.getText("enterSearch"))-2] // 去掉": "后缀
			return m, nil
		}
		m.logUserAction(fmt.Sprintf("搜索股票: %s", m.searchInput))
		m.message = m.getText("searching")
		m.searchResult = getStockInfo(m.searchInput)
		if m.searchResult == nil || m.searchResult.Name == "" {
			m.logUserAction(fmt.Sprintf("搜索失败: %s", m.searchInput))
			m.message = fmt.Sprintf(m.getText("searchNotFound"), m.searchInput)
			return m, nil
		}
		m.logUserAction(fmt.Sprintf("搜索成功: %s (%s)", m.searchResult.Name, m.searchResult.Symbol))

		// 如果是从自选列表进入的搜索，跳转到确认页面
		if m.searchFromWatchlist {
			m.state = WatchlistSearchConfirm
		} else {
			m.state = SearchResultWithActions
		}
		m.message = ""
		return m, nil
	case "backspace":
		if len(m.searchInput) > 0 {
			// 正确处理多字节字符（如中文）的删除
			runes := []rune(m.searchInput)
			if len(runes) > 0 {
				m.searchInput = string(runes[:len(runes)-1])
			}
		}
	default:
		// 改进的输入处理：支持多字节字符（如中文）
		str := msg.String()
		if len(str) > 0 && str != "\n" && str != "\r" && !isControlKey(str) {
			m.searchInput += str
		}
	}
	return m, nil
}

func (m *Model) handleSearchResult(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "r":
		m.state = SearchingStock
		m.searchFromWatchlist = false
		m.message = ""
		return m, nil
	}
	return m, nil
}

func (m *Model) viewSearchingStock() string {
	s := m.getText("searchTitle") + "\n\n"
	s += m.getText("enterSearch") + m.searchInput + "_\n\n"
	s += m.getText("searchFormats") + "\n\n"
	s += m.getText("searchHelp") + "\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

func (m *Model) viewSearchResult() string {
	s := m.getText("detailTitle") + "\n\n"

	if m.searchResult == nil {
		s += m.getText("noInfo") + "\n"
		s += "\n" + m.getText("detailHelp") + "\n"
		return s
	}

	// 创建横向表格显示股票详细信息
	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 构建表头和数据行
	var headers []interface{}
	var values []interface{}

	// 基本信息
	if m.language == Chinese {
		headers = append(headers, "股票代码", "股票名称", "现价")
	} else {
		headers = append(headers, "Code", "Name", "Price")
	}
	values = append(values, m.searchResult.Symbol, m.searchResult.Name, m.formatPriceWithColorLang(m.searchResult.Price, m.searchResult.PrevClose))

	// 昨收价
	if m.searchResult.PrevClose > 0 {
		if m.language == Chinese {
			headers = append(headers, "昨收价")
		} else {
			headers = append(headers, "Prev Close")
		}
		values = append(values, fmt.Sprintf("%.3f", m.searchResult.PrevClose))
	}

	// 价格信息（有数据时才显示）
	if m.searchResult.StartPrice > 0 {
		if m.language == Chinese {
			headers = append(headers, "开盘价")
		} else {
			headers = append(headers, "Open")
		}
		values = append(values, m.formatPriceWithColorLang(m.searchResult.StartPrice, m.searchResult.PrevClose))
	}
	if m.searchResult.MaxPrice > 0 {
		if m.language == Chinese {
			headers = append(headers, "最高价")
		} else {
			headers = append(headers, "High")
		}
		values = append(values, m.formatPriceWithColorLang(m.searchResult.MaxPrice, m.searchResult.PrevClose))
	}
	if m.searchResult.MinPrice > 0 {
		if m.language == Chinese {
			headers = append(headers, "最低价")
		} else {
			headers = append(headers, "Low")
		}
		values = append(values, m.formatPriceWithColorLang(m.searchResult.MinPrice, m.searchResult.PrevClose))
	}

	// 涨跌信息
	if m.searchResult.Change != 0 {
		if m.language == Chinese {
			headers = append(headers, "涨跌额")
		} else {
			headers = append(headers, "Change")
		}
		changeStr := m.formatProfitWithColorZeroLang(m.searchResult.Change)
		values = append(values, changeStr)
	}
	if m.searchResult.ChangePercent != 0 {
		if m.language == Chinese {
			headers = append(headers, "今日涨幅")
		} else {
			headers = append(headers, "Change %")
		}
		changePercentStr := m.formatProfitRateWithColorZeroLang(m.searchResult.ChangePercent)
		values = append(values, changePercentStr)
	}

	// 换手率
	if m.searchResult.TurnoverRate > 0 {
		if m.language == Chinese {
			headers = append(headers, "换手率")
		} else {
			headers = append(headers, "Turnover")
		}
		values = append(values, fmt.Sprintf("%.2f%%", m.searchResult.TurnoverRate))
	}

	// 买入量（成交量）
	if m.searchResult.Volume > 0 {
		if m.language == Chinese {
			headers = append(headers, "成交量")
		} else {
			headers = append(headers, "Volume")
		}
		volumeStr := formatVolume(m.searchResult.Volume)
		values = append(values, volumeStr)
	}

	// 添加表头和数据行
	t.AppendHeader(table.Row(headers))
	t.AppendRow(table.Row(values))

	s += t.Render() + "\n\n"
	s += m.getText("detailHelp") + "\n"

	return s
}

func formatVolume(volume int64) string {
	if volume >= 1000000000 {
		return fmt.Sprintf("%.2f十亿", float64(volume)/1000000000)
	} else if volume >= 100000000 {
		return fmt.Sprintf("%.2f亿", float64(volume)/100000000)
	} else if volume >= 10000 {
		return fmt.Sprintf("%.2f万", float64(volume)/10000)
	} else {
		return fmt.Sprintf("%d", volume)
	}
}

// 检查是否为控制键
func isControlKey(str string) bool {
	if len(str) == 0 {
		return true
	}

	// 检查常见的控制键序列
	controlKeys := []string{
		"ctrl+c", "ctrl+d", "ctrl+z", "ctrl+l", "ctrl+r",
		"alt+", "cmd+", "shift+", "ctrl+",
		"up", "down", "left", "right",
		"home", "end", "pgup", "pgdown",
		"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f11", "f12",
		"insert", "delete", "tab",
	}

	for _, key := range controlKeys {
		if strings.HasPrefix(strings.ToLower(str), key) {
			return true
		}
	}

	// 检查单个字符的控制字符（ASCII < 32，除了可打印字符）
	if len(str) == 1 {
		r := rune(str[0])
		if r < 32 && r != '\t' {
			return true
		}
	}

	return false
}

func (m *Model) handleLanguageSelection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = MainMenu
		m.message = "" // 清除消息
		return m, nil
	case "up", "k", "w":
		if m.languageCursor > 0 {
			m.languageCursor--
		}
	case "down", "j", "s":
		if m.languageCursor < 1 { // 0: Chinese, 1: English
			m.languageCursor++
		}
	case "enter", " ":
		// 选择语言
		if m.languageCursor == 0 {
			m.language = Chinese
			m.config.System.Language = "zh"
		} else {
			m.language = English
			m.config.System.Language = "en"
		}
		// 保存配置到文件
		if err := saveConfig(m.config); err != nil && m.debugMode {
			m.message = fmt.Sprintf("Warning: Failed to save config: %v", err)
		}
		// 更新菜单项
		m.menuItems = m.getMenuItems()
		m.state = MainMenu
		m.message = ""
		return m, nil
	}
	return m, nil
}

func (m *Model) viewLanguageSelection() string {
	s := m.getText("languageTitle") + "\n\n"
	s += m.getText("selectLanguage") + "\n\n"

	// 语言选项
	languages := []string{
		"中文简体",
		"English",
	}

	for i, lang := range languages {
		prefix := "  "
		if i == m.languageCursor {
			prefix = "► "
		}
		s += fmt.Sprintf("%s%s\n", prefix, lang)
	}

	s += "\n" + m.getText("languageHelp") + "\n"

	return s
}

// ========== 自选股票相关功能 ==========

// 加载自选股票列表
func loadWatchlist() Watchlist {
	data, err := os.ReadFile(watchlistFile)
	if err != nil {
		return Watchlist{Stocks: []WatchlistStock{}}
	}

	var watchlist Watchlist
	err = json.Unmarshal(data, &watchlist)
	if err != nil {
		return Watchlist{Stocks: []WatchlistStock{}}
	}
	return watchlist
}

// 保存自选股票列表
func (m *Model) saveWatchlist() {
	data, err := json.MarshalIndent(m.watchlist, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(watchlistFile, data, 0644)
}

// 检查股票是否已在自选列表中
func (m *Model) isStockInWatchlist(code string) bool {
	for _, stock := range m.watchlist.Stocks {
		if stock.Code == code {
			return true
		}
	}
	return false
}

// 添加股票到自选列表
func (m *Model) addToWatchlist(code, name string) bool {
	if m.isStockInWatchlist(code) {
		return false // 已在列表中
	}

	watchStock := WatchlistStock{
		Code: code,
		Name: name,
	}
	m.watchlist.Stocks = append(m.watchlist.Stocks, watchStock)
	m.saveWatchlist()
	return true
}

// 从自选列表删除股票
func (m *Model) removeFromWatchlist(index int) {
	if index >= 0 && index < len(m.watchlist.Stocks) {
		m.watchlist.Stocks = append(m.watchlist.Stocks[:index], m.watchlist.Stocks[index+1:]...)
		m.saveWatchlist()
	}
}

// ========== 搜索结果带操作按钮处理 ==========

func (m *Model) handleSearchResultWithActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "r":
		m.state = SearchingStock
		m.searchFromWatchlist = false
		m.message = ""
		return m, nil
	case "1":
		// 添加到自选列表并跳转到自选页面
		if m.searchResult != nil {
			if m.addToWatchlist(m.searchResult.Symbol, m.searchResult.Name) {
				m.message = fmt.Sprintf(m.getText("addWatchSuccess"), m.searchResult.Name, m.searchResult.Symbol)
			} else {
				m.message = fmt.Sprintf(m.getText("alreadyInWatch"), m.searchResult.Symbol)
			}
			// 跳转到自选列表页面
			m.state = WatchlistViewing
			// 设置滚动位置和光标到显示前N条股票
			if len(m.watchlist.Stocks) > 0 {
				maxWatchlistLines := m.config.Display.MaxLines
				if len(m.watchlist.Stocks) > maxWatchlistLines {
					// 显示前N条：滚动位置设置为显示从索引0开始的N条
					m.watchlistScrollPos = len(m.watchlist.Stocks) - maxWatchlistLines
					m.watchlistCursor = 0 // 光标指向第一个股票（索引0）
				} else {
					// 股票数量不超过显示行数，显示全部
					m.watchlistScrollPos = 0
					m.watchlistCursor = 0
				}
			}
			m.cursor = 0
			m.lastUpdate = time.Now()
		}
		return m, m.tickCmd()
	case "2":
		// 添加到持股列表（进入添加流程）
		if m.searchResult != nil {
			m.state = AddingStock
			m.addingStep = 1 // 跳过代码输入，直接到成本价输入
			m.tempCode = m.searchResult.Symbol
			m.stockInfo = &StockData{
				Symbol: m.searchResult.Symbol,
				Name:   m.searchResult.Name,
				Price:  m.searchResult.Price,
			}
			m.input = ""
			m.message = ""
			m.fromSearch = true // 标记从搜索结果添加
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) viewSearchResultWithActions() string {
	s := m.getText("detailTitle") + "\n\n"

	if m.searchResult == nil {
		s += m.getText("noInfo") + "\n"
		s += "\n" + m.getText("actionHelp") + "\n"
		return s
	}

	// 复用原有的搜索结果显示逻辑
	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 构建表头和数据行
	var headers []interface{}
	var values []interface{}

	// 基本信息
	if m.language == Chinese {
		headers = append(headers, "股票代码", "股票名称", "现价")
	} else {
		headers = append(headers, "Code", "Name", "Price")
	}
	values = append(values, m.searchResult.Symbol, m.searchResult.Name, m.formatPriceWithColorLang(m.searchResult.Price, m.searchResult.PrevClose))

	// 昨收价
	if m.searchResult.PrevClose > 0 {
		if m.language == Chinese {
			headers = append(headers, "昨收价")
		} else {
			headers = append(headers, "Prev Close")
		}
		values = append(values, fmt.Sprintf("%.3f", m.searchResult.PrevClose))
	}

	// 价格信息（有数据时才显示）
	if m.searchResult.StartPrice > 0 {
		if m.language == Chinese {
			headers = append(headers, "开盘价")
		} else {
			headers = append(headers, "Open")
		}
		values = append(values, m.formatPriceWithColorLang(m.searchResult.StartPrice, m.searchResult.PrevClose))
	}
	if m.searchResult.MaxPrice > 0 {
		if m.language == Chinese {
			headers = append(headers, "最高价")
		} else {
			headers = append(headers, "High")
		}
		values = append(values, m.formatPriceWithColorLang(m.searchResult.MaxPrice, m.searchResult.PrevClose))
	}
	if m.searchResult.MinPrice > 0 {
		if m.language == Chinese {
			headers = append(headers, "最低价")
		} else {
			headers = append(headers, "Low")
		}
		values = append(values, m.formatPriceWithColorLang(m.searchResult.MinPrice, m.searchResult.PrevClose))
	}

	// 涨跌信息
	if m.searchResult.Change != 0 {
		if m.language == Chinese {
			headers = append(headers, "涨跌额")
		} else {
			headers = append(headers, "Change")
		}
		changeStr := m.formatProfitWithColorZeroLang(m.searchResult.Change)
		values = append(values, changeStr)
	}
	if m.searchResult.ChangePercent != 0 {
		if m.language == Chinese {
			headers = append(headers, "今日涨幅")
		} else {
			headers = append(headers, "Change %")
		}
		changePercentStr := m.formatProfitRateWithColorZeroLang(m.searchResult.ChangePercent)
		values = append(values, changePercentStr)
	}

	// 换手率
	if m.searchResult.TurnoverRate > 0 {
		if m.language == Chinese {
			headers = append(headers, "换手率")
		} else {
			headers = append(headers, "Turnover")
		}
		values = append(values, fmt.Sprintf("%.2f%%", m.searchResult.TurnoverRate))
	}

	// 买入量（成交量）
	if m.searchResult.Volume > 0 {
		if m.language == Chinese {
			headers = append(headers, "成交量")
		} else {
			headers = append(headers, "Volume")
		}
		volumeStr := formatVolume(m.searchResult.Volume)
		values = append(values, volumeStr)
	}

	// 添加表头和数据行
	t.AppendHeader(table.Row(headers))
	t.AppendRow(table.Row(values))

	s += t.Render() + "\n\n"

	// 操作按钮提示
	s += m.getText("actionHelp") + "\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

// ========== 自选股票查看处理 ==========

func (m *Model) handleWatchlistViewing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "m":
		m.state = MainMenu
		m.message = ""
		return m, nil
	case "d":
		// 进入删除模式
		if len(m.watchlist.Stocks) > 0 {
			m.state = WatchlistRemoving
			m.cursor = 0
		}
		return m, nil
	case "a":
		// 跳转到股票搜索页面
		m.logUserAction("从自选列表跳转到股票搜索页面")
		m.state = SearchingStock
		m.searchInput = ""
		m.searchResult = nil
		m.searchFromWatchlist = true
		m.message = ""
		return m, nil
	case "up", "k", "w":
		if m.watchlistCursor > 0 {
			m.watchlistCursor--
		}
		return m, nil
	case "down", "j", "s":
		if m.watchlistCursor < len(m.watchlist.Stocks)-1 {
			m.watchlistCursor++
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) viewWatchlistViewing() string {
	s := m.getText("watchlistTitle") + "\n"
	s += fmt.Sprintf(m.getText("updateTime"), m.lastUpdate.Format("2006-01-02 15:04:05")) + "\n\n"

	if len(m.watchlist.Stocks) == 0 {
		s += m.getText("emptyWatchlist") + "\n\n"
		s += m.getText("addToWatchFirst") + "\n\n"
		s += m.getText("watchlistHelp") + "\n"
		return s
	}

	// 显示滚动信息
	totalWatchStocks := len(m.watchlist.Stocks)
	maxWatchlistLines := m.config.Display.MaxLines
	if totalWatchStocks > 0 {
		currentPos := m.watchlistCursor + 1 // 显示从1开始的位置
		if m.language == Chinese {
			s += fmt.Sprintf("⭐ 自选列表 (%d/%d) [↑/↓:翻页]\n", currentPos, totalWatchStocks)
		} else {
			s += fmt.Sprintf("⭐ Watchlist (%d/%d) [↑/↓:scroll]\n", currentPos, totalWatchStocks)
		}
		s += "\n"
	}

	// 创建表格显示自选股票列表
	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 获取本地化的表头
	if m.language == Chinese {
		t.AppendHeader(table.Row{"", "代码", "名称", "现价", "昨收价", "开盘", "最高", "最低", "今日涨幅", "换手率", "成交量"})
	} else {
		t.AppendHeader(table.Row{"", "Code", "Name", "Price", "PrevClose", "Open", "High", "Low", "Today%", "Turnover", "Volume"})
	}

	// 计算要显示的自选股票范围
	watchStocks := m.watchlist.Stocks
	endIndex := len(watchStocks) - m.watchlistScrollPos
	startIndex := endIndex - maxWatchlistLines
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex > len(watchStocks) {
		endIndex = len(watchStocks)
	}

	for i := startIndex; i < endIndex; i++ {
		watchStock := watchStocks[i]
		// 获取实时股价数据
		stockData := getStockPrice(watchStock.Code)
		if stockData != nil {
			// 计算今日涨幅：应该基于昨收价，而不是开盘价
			var todayChangeStr string
			// 使用change_percent字段，这是基于昨收价计算的涨跌幅
			if stockData.ChangePercent != 0 {
				todayChangeStr = m.formatProfitRateWithColorZeroLang(stockData.ChangePercent)
			} else {
				todayChangeStr = "-"
			}

			// 换手率显示
			var turnoverStr string
			if stockData.TurnoverRate > 0 {
				turnoverStr = fmt.Sprintf("%.2f%%", stockData.TurnoverRate)
			} else {
				turnoverStr = "-"
			}

			// 成交量显示
			volumeStr := formatVolume(stockData.Volume)

			// 光标列 - 检查光标是否在当前可见范围内且指向此行
			cursorCol := ""
			if m.watchlistCursor >= startIndex && m.watchlistCursor < endIndex && i == m.watchlistCursor {
				cursorCol = "►"
			}

			t.AppendRow(table.Row{
				cursorCol,
				watchStock.Code,
				watchStock.Name,
				m.formatPriceWithColorLang(stockData.Price, stockData.PrevClose),
				fmt.Sprintf("%.3f", stockData.PrevClose),
				m.formatPriceWithColorLang(stockData.StartPrice, stockData.PrevClose),
				m.formatPriceWithColorLang(stockData.MaxPrice, stockData.PrevClose),
				m.formatPriceWithColorLang(stockData.MinPrice, stockData.PrevClose),
				todayChangeStr,
				turnoverStr,
				volumeStr,
			})
		} else {
			// 如果无法获取数据，显示基本信息
			// 光标列 - 检查光标是否在当前可见范围内且指向此行
			cursorCol := ""
			if m.watchlistCursor >= startIndex && m.watchlistCursor < endIndex && i == m.watchlistCursor {
				cursorCol = "►"
			}

			t.AppendRow(table.Row{
				cursorCol,
				watchStock.Code,
				watchStock.Name,
				"-",
				"-",
				"-",
				"-",
				"-",
				"-",
				"-",
				"-",
			})
		}

		// 在每个股票后添加分隔线（除了显示范围内的最后一个）
		if i < endIndex-1 {
			t.AppendSeparator()
		}
	}

	s += t.Render() + "\n"

	// 如果可以滚动，显示滚动指示
	if totalWatchStocks > maxWatchlistLines {
		s += "\n" + strings.Repeat("-", 80) + "\n"
		if m.watchlistScrollPos > 0 {
			if m.language == Chinese {
				s += "↑ 有更新的自选股票 (按↓查看)\n"
			} else {
				s += "↑ Newer watchlist stocks available (press ↓)\n"
			}
		}
		if m.watchlistScrollPos < totalWatchStocks-1 {
			if m.language == Chinese {
				s += "↓ 有更多历史自选股票 (按↑查看)\n"
			} else {
				s += "↓ More watchlist stocks available (press ↑)\n"
			}
		}
	}

	s += "\n" + m.getText("watchlistHelp") + "\n"

	if m.message != "" {
		s += "\n" + m.message + "\n"
	}

	return s
}

// ========== 自选股票删除处理 ==========

func (m *Model) handleWatchlistRemoving(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = WatchlistViewing
		m.message = ""
		m.lastUpdate = time.Now()
		return m, m.tickCmd()
	case "up", "k", "w":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j", "s":
		if m.cursor < len(m.watchlist.Stocks)-1 {
			m.cursor++
		}
	case "enter", " ":
		if len(m.watchlist.Stocks) > 0 {
			removedStock := m.watchlist.Stocks[m.cursor]
			m.removeFromWatchlist(m.cursor)
			m.state = WatchlistViewing
			m.lastUpdate = time.Now()
			m.message = fmt.Sprintf(m.getText("removeWatchSuccess"), removedStock.Name, removedStock.Code)
		}
		return m, m.tickCmd()
	}
	return m, nil
}

func (m *Model) viewWatchlistRemoving() string {
	s := m.getText("removeFromWatch") + "\n\n"

	if len(m.watchlist.Stocks) == 0 {
		s += m.getText("emptyWatchlist") + "\n\n" + m.getText("returnToMenuShort") + "\n"
		return s
	}

	s += m.getText("selectToRemoveWatch") + "\n\n"
	for i, stock := range m.watchlist.Stocks {
		prefix := "  "
		if i == m.cursor {
			prefix = "► "
		}
		s += fmt.Sprintf("%s%d. %s (%s)\n", prefix, i+1, stock.Name, stock.Code)
	}

	s += "\n" + m.getText("navHelp") + "\n"
	return s
}

func gbkToUtf8(data []byte) (string, error) {
	reader := transform.NewReader(strings.NewReader(string(data)), simplifiedchinese.GBK.NewDecoder())
	utf8Data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(utf8Data), nil
}

// ========== 自选股票搜索确认处理 ==========

func (m *Model) handleWatchlistSearchConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = WatchlistViewing
		m.searchFromWatchlist = false
		m.message = ""
		return m, nil
	case "enter":
		// 确认添加到自选列表
		if m.searchResult != nil {
			if m.addToWatchlist(m.searchResult.Symbol, m.searchResult.Name) {
				m.message = fmt.Sprintf(m.getText("addWatchSuccess"), m.searchResult.Name, m.searchResult.Symbol)
				m.logUserAction(fmt.Sprintf("添加到自选列表: %s (%s)", m.searchResult.Name, m.searchResult.Symbol))
			} else {
				m.message = fmt.Sprintf(m.getText("alreadyInWatch"), m.searchResult.Symbol)
			}
			m.state = WatchlistViewing
			m.searchFromWatchlist = false
			return m, m.tickCmd()
		}
		return m, nil
	case "r":
		// 重新搜索
		m.state = SearchingStock
		m.searchInput = ""
		m.searchResult = nil
		m.message = ""
		return m, nil
	}
	return m, nil
}

func (m *Model) viewWatchlistSearchConfirm() string {
	if m.searchResult == nil {
		return m.getText("searchNotFound")
	}

	s := m.getText("searchTitle") + "\n\n"

	// 创建表格显示股票信息
	t := table.NewWriter()
	t.SetStyle(table.StyleLight)

	// 设置表头
	if m.language == Chinese {
		t.AppendHeader(table.Row{"名称", "现价", "昨收价", "开盘", "最高", "最低", "今日涨幅", "换手率", "成交量"})
	} else {
		t.AppendHeader(table.Row{"Name", "Price", "PrevClose", "Open", "High", "Low", "Today%", "Turnover", "Volume"})
	}

	// 构建数据行
	var values []interface{}

	// 名称
	values = append(values, m.searchResult.Name)

	// 现价 (带颜色)
	priceStr := m.formatPriceWithColorLang(m.searchResult.Price, m.searchResult.PrevClose)
	values = append(values, priceStr)

	// 昨收价
	values = append(values, fmt.Sprintf("%.3f", m.searchResult.PrevClose))

	// 开盘价
	if m.searchResult.StartPrice > 0 {
		openStr := m.formatPriceWithColorLang(m.searchResult.StartPrice, m.searchResult.PrevClose)
		values = append(values, openStr)
	} else {
		values = append(values, "-")
	}

	// 最高价
	if m.searchResult.MaxPrice > 0 {
		highStr := m.formatPriceWithColorLang(m.searchResult.MaxPrice, m.searchResult.PrevClose)
		values = append(values, highStr)
	} else {
		values = append(values, "-")
	}

	// 最低价
	if m.searchResult.MinPrice > 0 {
		lowStr := m.formatPriceWithColorLang(m.searchResult.MinPrice, m.searchResult.PrevClose)
		values = append(values, lowStr)
	} else {
		values = append(values, "-")
	}

	// 今日涨幅
	if m.searchResult.ChangePercent != 0 {
		changePercentStr := m.formatProfitRateWithColorZeroLang(m.searchResult.ChangePercent)
		values = append(values, changePercentStr)
	} else {
		values = append(values, "-")
	}

	// 换手率
	if m.searchResult.TurnoverRate > 0 {
		values = append(values, fmt.Sprintf("%.2f%%", m.searchResult.TurnoverRate))
	} else {
		values = append(values, "-")
	}

	// 成交量
	if m.searchResult.Volume > 0 {
		if m.searchResult.Volume >= 100000000 { // 大于等于1亿
			values = append(values, fmt.Sprintf("%.2f亿", float64(m.searchResult.Volume)/100000000))
		} else if m.searchResult.Volume >= 10000 { // 大于等于1万
			values = append(values, fmt.Sprintf("%.2f万", float64(m.searchResult.Volume)/10000))
		} else {
			values = append(values, fmt.Sprintf("%d", m.searchResult.Volume))
		}
	} else {
		values = append(values, "-")
	}

	t.AppendRow(values)

	s += t.Render() + "\n\n"

	if m.language == Chinese {
		s += "按回车键添加到自选列表，ESC键返回，R键重新搜索\n"
	} else {
		s += "Press Enter to add to watchlist, ESC to return, R to search again\n"
	}

	return s
}
