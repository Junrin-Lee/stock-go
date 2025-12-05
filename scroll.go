package main

// ============================================================================
// 持股列表滚动控制
// ============================================================================

// scrollPortfolioUp 向上滚动持股列表
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

// scrollPortfolioDown 向下滚动持股列表
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

// ============================================================================
// 自选列表滚动控制
// ============================================================================

// scrollWatchlistUp 向上滚动自选列表
func (m *Model) scrollWatchlistUp() {
	// 向上翻页：显示更早的股票，光标也向上移动
	if m.watchlistCursor > 0 {
		m.watchlistCursor--
		// 获取一次过滤后的列表，避免重复调用
		filteredStocks := m.getFilteredWatchlist()
		m.adjustWatchlistScroll(filteredStocks)
	}
}

// scrollWatchlistDown 向下滚动自选列表
func (m *Model) scrollWatchlistDown() {
	// 获取一次过滤后的列表，避免重复调用
	filteredStocks := m.getFilteredWatchlist()
	// 向下翻页：显示更新的股票，光标也向下移动
	if m.watchlistCursor < len(filteredStocks)-1 {
		m.watchlistCursor++
		m.adjustWatchlistScroll(filteredStocks)
	}
}
