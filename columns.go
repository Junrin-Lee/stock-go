package main

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
)

// ColumnID - 列的唯一标识符
type ColumnID string

// Portfolio列ID常量
const (
	ColCursor         ColumnID = "cursor"
	ColCode           ColumnID = "code"
	ColName           ColumnID = "name"
	ColPrevClose      ColumnID = "prev_close"
	ColOpen           ColumnID = "open"
	ColHigh           ColumnID = "high"
	ColLow            ColumnID = "low"
	ColPrice          ColumnID = "price"
	ColCost           ColumnID = "cost"
	ColQuantity       ColumnID = "quantity"
	ColTodayChange    ColumnID = "today_change"
	ColPositionProfit ColumnID = "position_profit"
	ColProfitRate     ColumnID = "profit_rate"
	ColMarketValue    ColumnID = "market_value"

	// Watchlist特有列ID
	ColTag      ColumnID = "tag"
	ColTurnover ColumnID = "turnover"
	ColVolume   ColumnID = "volume"
)

// ColumnMetadata - 列的元数据
type ColumnMetadata struct {
	ID         ColumnID   // 列ID
	I18nKey    string     // 国际化翻译键
	IsRequired bool       // 是否为必须列（不可隐藏）
	SortField  *SortField // 关联的排序字段（nil表示不可排序）
}

// ColumnRegistry - 列注册表
type ColumnRegistry struct {
	portfolioColumns map[ColumnID]*ColumnMetadata
	watchlistColumns map[ColumnID]*ColumnMetadata
}

// 全局列注册表实例
var columnRegistry *ColumnRegistry

// initColumnRegistry - 初始化列注册表
func initColumnRegistry() {
	columnRegistry = &ColumnRegistry{
		portfolioColumns: makePortfolioColumnRegistry(),
		watchlistColumns: makeWatchlistColumnRegistry(),
	}
}

// makePortfolioColumnRegistry - 创建Portfolio列注册表
func makePortfolioColumnRegistry() map[ColumnID]*ColumnMetadata {
	// 创建排序字段指针（用于元数据引用）
	sortByCode := SortByCode
	sortByName := SortByName
	sortByPrice := SortByPrice
	sortByCostPrice := SortByCostPrice
	sortByQuantity := SortByQuantity
	sortByChangePercent := SortByChangePercent
	sortByTotalProfit := SortByTotalProfit
	sortByProfitRate := SortByProfitRate
	sortByMarketValue := SortByMarketValue

	return map[ColumnID]*ColumnMetadata{
		ColCursor: {
			ID:         ColCursor,
			I18nKey:    "", // 光标列无需翻译
			IsRequired: true,
			SortField:  nil, // 不可排序
		},
		ColCode: {
			ID:         ColCode,
			I18nKey:    "col.code",
			IsRequired: true,
			SortField:  &sortByCode,
		},
		ColName: {
			ID:         ColName,
			I18nKey:    "col.name",
			IsRequired: true,
			SortField:  &sortByName,
		},
		ColPrevClose: {
			ID:         ColPrevClose,
			I18nKey:    "col.prev_close",
			IsRequired: false,
			SortField:  nil,
		},
		ColOpen: {
			ID:         ColOpen,
			I18nKey:    "col.open",
			IsRequired: false,
			SortField:  nil,
		},
		ColHigh: {
			ID:         ColHigh,
			I18nKey:    "col.high",
			IsRequired: false,
			SortField:  nil,
		},
		ColLow: {
			ID:         ColLow,
			I18nKey:    "col.low",
			IsRequired: false,
			SortField:  nil,
		},
		ColPrice: {
			ID:         ColPrice,
			I18nKey:    "col.price",
			IsRequired: true,
			SortField:  &sortByPrice,
		},
		ColCost: {
			ID:         ColCost,
			I18nKey:    "col.cost",
			IsRequired: false,
			SortField:  &sortByCostPrice,
		},
		ColQuantity: {
			ID:         ColQuantity,
			I18nKey:    "col.quantity",
			IsRequired: false,
			SortField:  &sortByQuantity,
		},
		ColTodayChange: {
			ID:         ColTodayChange,
			I18nKey:    "col.today_change",
			IsRequired: false,
			SortField:  &sortByChangePercent,
		},
		ColPositionProfit: {
			ID:         ColPositionProfit,
			I18nKey:    "col.position_profit",
			IsRequired: false,
			SortField:  &sortByTotalProfit,
		},
		ColProfitRate: {
			ID:         ColProfitRate,
			I18nKey:    "col.profit_rate",
			IsRequired: false,
			SortField:  &sortByProfitRate,
		},
		ColMarketValue: {
			ID:         ColMarketValue,
			I18nKey:    "col.market_value",
			IsRequired: false,
			SortField:  &sortByMarketValue,
		},
	}
}

// makeWatchlistColumnRegistry - 创建Watchlist列注册表
func makeWatchlistColumnRegistry() map[ColumnID]*ColumnMetadata {
	// 创建排序字段指针
	sortByTag := SortByTag
	sortByCode := SortByCode
	sortByName := SortByName
	sortByPrice := SortByPrice
	sortByChangePercent := SortByChangePercent
	sortByTurnoverRate := SortByTurnoverRate
	sortByVolume := SortByVolume

	return map[ColumnID]*ColumnMetadata{
		ColCursor: {
			ID:         ColCursor,
			I18nKey:    "",
			IsRequired: true,
			SortField:  nil,
		},
		ColTag: {
			ID:         ColTag,
			I18nKey:    "col.tag",
			IsRequired: true,
			SortField:  &sortByTag,
		},
		ColCode: {
			ID:         ColCode,
			I18nKey:    "col.code",
			IsRequired: true,
			SortField:  &sortByCode,
		},
		ColName: {
			ID:         ColName,
			I18nKey:    "col.name",
			IsRequired: true,
			SortField:  &sortByName,
		},
		ColPrice: {
			ID:         ColPrice,
			I18nKey:    "col.price",
			IsRequired: true,
			SortField:  &sortByPrice,
		},
		ColPrevClose: {
			ID:         ColPrevClose,
			I18nKey:    "col.prev_close",
			IsRequired: false,
			SortField:  nil,
		},
		ColOpen: {
			ID:         ColOpen,
			I18nKey:    "col.open",
			IsRequired: false,
			SortField:  nil,
		},
		ColHigh: {
			ID:         ColHigh,
			I18nKey:    "col.high",
			IsRequired: false,
			SortField:  nil,
		},
		ColLow: {
			ID:         ColLow,
			I18nKey:    "col.low",
			IsRequired: false,
			SortField:  nil,
		},
		ColTodayChange: {
			ID:         ColTodayChange,
			I18nKey:    "col.today_change",
			IsRequired: false,
			SortField:  &sortByChangePercent,
		},
		ColTurnover: {
			ID:         ColTurnover,
			I18nKey:    "col.turnover",
			IsRequired: false,
			SortField:  &sortByTurnoverRate,
		},
		ColVolume: {
			ID:         ColVolume,
			I18nKey:    "col.volume",
			IsRequired: false,
			SortField:  &sortByVolume,
		},
	}
}

// GetPortfolioColumns - 获取Portfolio的活跃列列表
func (m *Model) GetPortfolioColumns() []*ColumnMetadata {
	configuredColumns := m.config.Display.PortfolioColumns
	return buildColumnList(configuredColumns, columnRegistry.portfolioColumns)
}

// GetWatchlistColumns - 获取Watchlist的活跃列列表
func (m *Model) GetWatchlistColumns() []*ColumnMetadata {
	configuredColumns := m.config.Display.WatchlistColumns
	return buildColumnList(configuredColumns, columnRegistry.watchlistColumns)
}

// buildColumnList - 从配置构建列元数据列表
func buildColumnList(configIDs []string, registry map[ColumnID]*ColumnMetadata) []*ColumnMetadata {
	var result []*ColumnMetadata
	for _, idStr := range configIDs {
		id := ColumnID(idStr)
		if meta, exists := registry[id]; exists {
			result = append(result, meta)
		} else {
			// 静默忽略无效列ID（在debug模式下会记录）
			debugPrint("debug.config.invalidColumn", idStr)
		}
	}
	return result
}

// GeneratePortfolioHeader - 生成Portfolio表头（含排序指示器）
func (m *Model) GeneratePortfolioHeader() table.Row {
	columns := m.GetPortfolioColumns()
	header := make(table.Row, len(columns))

	for i, col := range columns {
		// 基础列名
		if col.I18nKey == "" {
			header[i] = "" // cursor列无名称
		} else {
			header[i] = m.getText(col.I18nKey)
		}

		// 添加排序指示器
		if col.SortField != nil &&
			m.portfolioIsSorted &&
			*col.SortField == m.portfolioSortField {
			indicator := "↑"
			if m.portfolioSortDirection == SortDesc {
				indicator = "↓"
			}
			header[i] = fmt.Sprintf("%s %s", header[i], indicator)
		}
	}

	return header
}

// GeneratePortfolioRow - 生成Portfolio数据行
func (m *Model) GeneratePortfolioRow(stock *Stock, rowIndex, startIndex, endIndex int) table.Row {
	columns := m.GetPortfolioColumns()
	row := make(table.Row, len(columns))

	for i, col := range columns {
		switch col.ID {
		case ColCursor:
			// 光标列特殊处理
			if m.portfolioCursor >= startIndex && m.portfolioCursor < endIndex && rowIndex == m.portfolioCursor {
				row[i] = "►"
			} else {
				row[i] = ""
			}
		case ColCode:
			row[i] = stock.Code
		case ColName:
			row[i] = stock.Name
		case ColPrevClose:
			row[i] = fmt.Sprintf("%.3f", stock.PrevClose)
		case ColOpen:
			row[i] = m.formatPriceWithColorLang(stock.StartPrice, stock.PrevClose)
		case ColHigh:
			row[i] = m.formatPriceWithColorLang(stock.MaxPrice, stock.PrevClose)
		case ColLow:
			row[i] = m.formatPriceWithColorLang(stock.MinPrice, stock.PrevClose)
		case ColPrice:
			row[i] = m.formatPriceWithColorLang(stock.Price, stock.PrevClose)
		case ColCost:
			row[i] = fmt.Sprintf("%.3f", stock.CostPrice)
		case ColQuantity:
			row[i] = stock.Quantity
		case ColTodayChange:
			if stock.Price > 0 && stock.PrevClose > 0 {
				row[i] = m.formatProfitRateWithColorZeroLang(stock.ChangePercent)
			} else {
				row[i] = "-"
			}
		case ColPositionProfit:
			if stock.Price > 0 {
				profit := stock.CalculatePositionProfit()
				row[i] = m.formatProfitWithColorZeroLang(profit)
			} else {
				row[i] = "-"
			}
		case ColProfitRate:
			if stock.Price > 0 && stock.CostPrice > 0 {
				profitRate := ((stock.Price - stock.CostPrice) / stock.CostPrice) * 100
				row[i] = m.formatProfitRateWithColorZeroLang(profitRate)
			} else {
				row[i] = "-"
			}
		case ColMarketValue:
			marketValue := float64(stock.Quantity) * stock.Price
			row[i] = fmt.Sprintf("%.2f", marketValue)
		default:
			row[i] = "-"
		}
	}

	return row
}

// GeneratePortfolioTotalRow - 生成Portfolio总计行
func (m *Model) GeneratePortfolioTotalRow(totalProfit, totalProfitRate, totalMarketValue float64) table.Row {
	columns := m.GetPortfolioColumns()
	row := make(table.Row, len(columns))

	for i, col := range columns {
		switch col.ID {
		case ColName:
			row[i] = m.getText("total")
		case ColPositionProfit:
			row[i] = m.formatProfitWithColorLang(totalProfit)
		case ColProfitRate:
			row[i] = m.formatProfitRateWithColorLang(totalProfitRate)
		case ColMarketValue:
			row[i] = fmt.Sprintf("%.2f", totalMarketValue)
		default:
			row[i] = ""
		}
	}

	return row
}

// GenerateWatchlistHeader - 生成Watchlist表头（含排序指示器）
func (m *Model) GenerateWatchlistHeader() table.Row {
	columns := m.GetWatchlistColumns()
	header := make(table.Row, len(columns))

	for i, col := range columns {
		// 基础列名
		if col.I18nKey == "" {
			header[i] = "" // cursor列无名称
		} else {
			header[i] = m.getText(col.I18nKey)
		}

		// 添加排序指示器
		if col.SortField != nil &&
			m.watchlistIsSorted &&
			*col.SortField == m.watchlistSortField {
			indicator := "↑"
			if m.watchlistSortDirection == SortDesc {
				indicator = "↓"
			}
			header[i] = fmt.Sprintf("%s %s", header[i], indicator)
		}
	}

	return header
}

// GenerateWatchlistRow - 生成Watchlist数据行
func (m *Model) GenerateWatchlistRow(watchStock *WatchlistStock, stockData *StockData, rowIndex, startIndex, endIndex int) table.Row {
	columns := m.GetWatchlistColumns()
	row := make(table.Row, len(columns))

	for i, col := range columns {
		switch col.ID {
		case ColCursor:
			// 光标列特殊处理
			if m.watchlistCursor >= startIndex && m.watchlistCursor < endIndex && rowIndex == m.watchlistCursor {
				row[i] = "►"
			} else {
				row[i] = ""
			}
		case ColTag:
			row[i] = watchStock.getTagsDisplay()
		case ColCode:
			row[i] = watchStock.Code
		case ColName:
			// Watchlist的name列需要portfolio高亮
			row[i] = m.formatStockNameWithPortfolioHighlight(watchStock.Name, watchStock.Code)
		case ColPrice:
			if stockData != nil && stockData.Price > 0 && stockData.PrevClose > 0 {
				row[i] = m.formatPriceWithColorLang(stockData.Price, stockData.PrevClose)
			} else {
				row[i] = "-"
			}
		case ColPrevClose:
			if stockData != nil {
				row[i] = fmt.Sprintf("%.3f", stockData.PrevClose)
			} else {
				row[i] = "-"
			}
		case ColOpen:
			if stockData != nil && stockData.Price > 0 && stockData.PrevClose > 0 {
				row[i] = m.formatPriceWithColorLang(stockData.StartPrice, stockData.PrevClose)
			} else {
				row[i] = "-"
			}
		case ColHigh:
			if stockData != nil && stockData.Price > 0 && stockData.PrevClose > 0 {
				row[i] = m.formatPriceWithColorLang(stockData.MaxPrice, stockData.PrevClose)
			} else {
				row[i] = "-"
			}
		case ColLow:
			if stockData != nil && stockData.Price > 0 && stockData.PrevClose > 0 {
				row[i] = m.formatPriceWithColorLang(stockData.MinPrice, stockData.PrevClose)
			} else {
				row[i] = "-"
			}
		case ColTodayChange:
			if stockData != nil && stockData.Price > 0 && stockData.PrevClose > 0 {
				row[i] = m.formatProfitRateWithColorZeroLang(stockData.ChangePercent)
			} else {
				row[i] = "-"
			}
		case ColTurnover:
			if stockData != nil {
				row[i] = fmt.Sprintf("%.2f%%", stockData.TurnoverRate)
			} else {
				row[i] = "-"
			}
		case ColVolume:
			if stockData != nil {
				row[i] = formatVolume(stockData.Volume)
			} else {
				row[i] = "-"
			}
		default:
			row[i] = "-"
		}
	}

	return row
}
