package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ============================================================================
// 股价缓存管理
// ============================================================================

// getStockPriceFromCache 从缓存获取股价数据（非阻塞）
func (m *Model) getStockPriceFromCache(symbol string) *StockData {
	m.stockPriceMutex.RLock()
	defer m.stockPriceMutex.RUnlock()
	if entry, exists := m.stockPriceCache[symbol]; exists {
		// 检查缓存是否过期（超过30秒）
		if time.Since(entry.UpdateTime) < 30*time.Second {
			return entry.Data
		}
	}
	// 如果缓存中没有数据或已过期，返回nil，触发异步更新
	return nil
}

// ============================================================================
// 股价异步更新
// ============================================================================

// startStockPriceUpdates 启动股价异步更新
func (m *Model) startStockPriceUpdates() tea.Cmd {
	// 检查是否需要开始新的更新周期
	if time.Since(m.stockPriceUpdateTime) < 5*time.Second {
		debugPrint("debug.cache.skipUpdate", time.Since(m.stockPriceUpdateTime))
		return nil // 还未到更新时间
	}

	// 收集所有需要更新的股票代码
	stockCodes := make([]string, 0)

	// 添加自选列表中的股票 - 注意：这里应该获取所有自选股票，而不是过滤后的
	for _, stock := range m.watchlist.Stocks {
		stockCodes = append(stockCodes, stock.Code)
	}

	// 添加持股列表中的股票
	for _, stock := range m.portfolio.Stocks {
		stockCodes = append(stockCodes, stock.Code)
	}

	if len(stockCodes) == 0 {
		debugPrint("debug.cache.noStocks")
		return nil
	}

	// 去重股票代码
	uniqueCodes := make(map[string]bool)
	var uniqueStockCodes []string
	for _, code := range stockCodes {
		if !uniqueCodes[code] {
			uniqueCodes[code] = true
			uniqueStockCodes = append(uniqueStockCodes, code)
		}
	}

	// 更新开始时间
	m.stockPriceUpdateTime = time.Now()

	debugPrint("debug.cache.startAsync", len(uniqueStockCodes))

	// 逐个发起异步获取请求
	var cmds []tea.Cmd
	for _, code := range uniqueStockCodes {
		// 标记正在更新
		m.stockPriceMutex.Lock()
		if entry, exists := m.stockPriceCache[code]; exists {
			entry.IsUpdating = true
		} else {
			m.stockPriceCache[code] = &StockPriceCacheEntry{
				Data:       nil,
				UpdateTime: time.Time{},
				IsUpdating: true,
			}
		}
		m.stockPriceMutex.Unlock()

		// 为每个股票添加一个延迟，避免同时请求太多
		delay := time.Duration(len(cmds)) * 100 * time.Millisecond
		// 修复闭包问题：将code变量复制到局部变量
		stockCode := code
		cmds = append(cmds, tea.Tick(delay, func(t time.Time) tea.Msg {
			// 直接在这里执行获取操作，而不是返回Command
			data := getStockPrice(stockCode)

			// 更新缓存
			m.stockPriceMutex.Lock()
			defer m.stockPriceMutex.Unlock()

			// 只有在成功获取数据时才更新缓存
			if data != nil && data.Price > 0 {
				m.stockPriceCache[stockCode] = &StockPriceCacheEntry{
					Data:       data,
					UpdateTime: time.Now(),
					IsUpdating: false,
				}
			} else {
				// 获取失败时，标记为不在更新状态，但不更新缓存，这样下次还会尝试获取
				if entry, exists := m.stockPriceCache[stockCode]; exists {
					entry.IsUpdating = false
				}
			}

			var err error
			if data == nil || data.Price <= 0 {
				err = fmt.Errorf("failed to get stock price for %s", stockCode)
			}

			return stockPriceUpdateMsg{
				Symbol: stockCode,
				Data:   data,
				Error:  err,
			}
		}))
	}

	return tea.Batch(cmds...)
}
