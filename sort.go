package main

import (
	"sort"
	"strings"
)

// StockSorter 股票排序接口
type StockSorter interface {
	SortPortfolio(stocks []Stock, field SortField, direction SortDirection)
	SortWatchlist(stocks []WatchlistStock, stockCache map[string]*StockPriceCacheEntry, field SortField, direction SortDirection)
}

// DefaultSorter 默认排序实现（使用Go标准库的sort包）
type DefaultSorter struct{}

// NewDefaultSorter 创建默认排序器
func NewDefaultSorter() *DefaultSorter {
	return &DefaultSorter{}
}

// SortPortfolio 排序持股列表
func (s *DefaultSorter) SortPortfolio(stocks []Stock, field SortField, direction SortDirection) {
	sort.Slice(stocks, func(i, j int) bool {
		var result bool
		
		switch field {
		case SortByCode:
			result = stocks[i].Code < stocks[j].Code
		case SortByName:
			result = stocks[i].Name < stocks[j].Name
		case SortByPrice:
			result = stocks[i].Price < stocks[j].Price
		case SortByCostPrice:
			result = stocks[i].CostPrice < stocks[j].CostPrice
		case SortByChange:
			result = stocks[i].Change < stocks[j].Change
		case SortByChangePercent:
			result = stocks[i].ChangePercent < stocks[j].ChangePercent
		case SortByQuantity:
			result = stocks[i].Quantity < stocks[j].Quantity
		case SortByTotalProfit:
			profitI := float64(stocks[i].Quantity) * (stocks[i].Price - stocks[i].CostPrice)
			profitJ := float64(stocks[j].Quantity) * (stocks[j].Price - stocks[j].CostPrice)
			result = profitI < profitJ
		case SortByProfitRate:
			rateI := (stocks[i].Price - stocks[i].CostPrice) / stocks[i].CostPrice * 100
			rateJ := (stocks[j].Price - stocks[j].CostPrice) / stocks[j].CostPrice * 100
			result = rateI < rateJ
		case SortByMarketValue:
			valueI := stocks[i].Price * float64(stocks[i].Quantity)
			valueJ := stocks[j].Price * float64(stocks[j].Quantity)
			result = valueI < valueJ
		default:
			result = stocks[i].Code < stocks[j].Code
		}
		
		if direction == SortDesc {
			result = !result
		}
		
		return result
	})
}

// SortWatchlist 排序自选列表（使用缓存的股价数据，避免API调用）
func (s *DefaultSorter) SortWatchlist(stocks []WatchlistStock, stockCache map[string]*StockPriceCacheEntry, field SortField, direction SortDirection) {
	sort.Slice(stocks, func(i, j int) bool {
		var result bool
		
		// 获取缓存的股价数据
		stockDataI := s.getStockDataFromCache(stocks[i].Code, stockCache)
		stockDataJ := s.getStockDataFromCache(stocks[j].Code, stockCache)
		
		switch field {
		case SortByCode:
			result = stocks[i].Code < stocks[j].Code
		case SortByName:
			result = stocks[i].Name < stocks[j].Name
		case SortByTag:
			tagsI := s.getTagsDisplay(stocks[i].Tags)
			tagsJ := s.getTagsDisplay(stocks[j].Tags)
			result = tagsI < tagsJ
		case SortByPrice:
			priceI := s.getPrice(stockDataI)
			priceJ := s.getPrice(stockDataJ)
			result = priceI < priceJ
		case SortByChangePercent:
			changeI := s.getChangePercent(stockDataI)
			changeJ := s.getChangePercent(stockDataJ)
			result = changeI < changeJ
		case SortByTurnoverRate:
			turnoverI := s.getTurnoverRate(stockDataI)
			turnoverJ := s.getTurnoverRate(stockDataJ)
			result = turnoverI < turnoverJ
		case SortByVolume:
			volumeI := s.getVolume(stockDataI)
			volumeJ := s.getVolume(stockDataJ)
			result = volumeI < volumeJ
		default:
			result = stocks[i].Code < stocks[j].Code
		}
		
		if direction == SortDesc {
			result = !result
		}
		
		return result
	})
}

// 辅助函数：从缓存获取股价数据
func (s *DefaultSorter) getStockDataFromCache(code string, stockCache map[string]*StockPriceCacheEntry) *StockData {
	if entry, exists := stockCache[code]; exists && entry.Data != nil {
		return entry.Data
	}
	return nil
}

// 辅助函数：获取价格
func (s *DefaultSorter) getPrice(data *StockData) float64 {
	if data != nil {
		return data.Price
	}
	return 0
}

// 辅助函数：获取涨跌幅
func (s *DefaultSorter) getChangePercent(data *StockData) float64 {
	if data != nil {
		return data.ChangePercent
	}
	return 0
}

// 辅助函数：获取换手率
func (s *DefaultSorter) getTurnoverRate(data *StockData) float64 {
	if data != nil {
		return data.TurnoverRate
	}
	return 0
}

// 辅助函数：获取成交量
func (s *DefaultSorter) getVolume(data *StockData) int64 {
	if data != nil {
		return data.Volume
	}
	return 0
}

// 辅助函数：获取标签显示文本
func (s *DefaultSorter) getTagsDisplay(tags []string) string {
	var validTags []string
	for _, tag := range tags {
		if tag != "" && tag != "-" {
			validTags = append(validTags, tag)
		}
	}
	
	if len(validTags) == 0 {
		return "-"
	}
	
	return strings.Join(validTags, ",")
}

// 性能优化的排序函数，供Model调用
func (m *Model) optimizedSortPortfolio(field SortField, direction SortDirection) {
	sorter := NewDefaultSorter()
	sorter.SortPortfolio(m.portfolio.Stocks, field, direction)
}

// updatePortfolioPricesFromCache 从缓存更新持股列表的价格数据
// 用于排序前确保价格数据是最新的
func (m *Model) updatePortfolioPricesFromCache() {
	for i := range m.portfolio.Stocks {
		stock := &m.portfolio.Stocks[i]
		stockData := m.getStockPriceFromCache(stock.Code)
		if stockData != nil {
			stock.Price = stockData.Price
			stock.Change = stockData.Change
			stock.ChangePercent = stockData.ChangePercent
			stock.StartPrice = stockData.StartPrice
			stock.MaxPrice = stockData.MaxPrice
			stock.MinPrice = stockData.MinPrice
			stock.PrevClose = stockData.PrevClose
		}
	}
}

func (m *Model) optimizedSortWatchlist(field SortField, direction SortDirection) {
	// 获取过滤后的股票列表
	filteredStocks := m.getFilteredWatchlist()
	
	// 使用高效排序（基于缓存数据，避免API调用）
	sorter := NewDefaultSorter()
	
	// 读取股价缓存
	m.stockPriceMutex.RLock()
	stockCacheCopy := make(map[string]*StockPriceCacheEntry)
	for k, v := range m.stockPriceCache {
		stockCacheCopy[k] = v
	}
	m.stockPriceMutex.RUnlock()
	
	// 执行排序（使用缓存数据）
	sorter.SortWatchlist(filteredStocks, stockCacheCopy, field, direction)
	
	// 将排序后的过滤列表更新回原列表
	// 如果没有过滤，直接使用排序结果
	if m.selectedTag == "" {
		m.watchlist.Stocks = filteredStocks
	} else {
		// 如果有过滤，需要将排序结果更新回原列表
		// 创建一个映射来快速查找排序后的位置
		sortedMap := make(map[string]int)
		for i, stock := range filteredStocks {
			sortedMap[stock.Code] = i
		}
		
		// 重新排列原列表，将过滤的股票按排序顺序放在前面
		var newStocks []WatchlistStock
		var remainingStocks []WatchlistStock
		
		// 先添加排序后的过滤股票
		newStocks = append(newStocks, filteredStocks...)
		
		// 再添加未过滤的股票
		for _, stock := range m.watchlist.Stocks {
			if _, exists := sortedMap[stock.Code]; !exists {
				remainingStocks = append(remainingStocks, stock)
			}
		}
		
		newStocks = append(newStocks, remainingStocks...)
		m.watchlist.Stocks = newStocks
	}
}