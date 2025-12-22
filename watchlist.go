package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ============================================================================
// 市场标签函数
// ============================================================================

// getMarketTagName 根据市场类型和语言获取标签名称（展示层使用）
func (m *Model) getMarketTagName(market MarketType) string {
	switch market {
	case MarketChina:
		return m.getText("marketTag.china")
	case MarketUS:
		return m.getText("marketTag.us")
	case MarketHongKong:
		return m.getText("marketTag.hongkong")
	}
	return "-"
}

// isMarketTag 判断标签是否为市场标签（用于迁移清理）
func isMarketTag(tag string) bool {
	marketTags := []string{"A股", "A-Share", "美股", "US Stock", "港股", "HK Stock"}
	for _, mt := range marketTags {
		if tag == mt {
			return true
		}
	}
	return false
}

// ============================================================================
// 标签管理函数
// ============================================================================

// renameTagForAllStocks 批量更新所有使用指定标签的股票，将旧标签替换为新标签
func (m *Model) renameTagForAllStocks(oldTag, newTag string) int {
	updatedCount := 0

	for i := range m.watchlist.Stocks {
		stock := &m.watchlist.Stocks[i]
		hasOldTag := false

		// 检查股票是否有旧标签
		for j, tag := range stock.Tags {
			if tag == oldTag {
				// 替换为新标签
				stock.Tags[j] = newTag
				hasOldTag = true
			}
		}

		if hasOldTag {
			updatedCount++
		}
	}

	return updatedCount
}

// getAvailableTags 获取所有可用的标签（包括市场标签）
func (m *Model) getAvailableTags() []string {
	tagMap := make(map[string]bool)

	// 添加所有市场标签
	for _, stock := range m.watchlist.Stocks {
		if stock.Market != "" {
			marketTag := m.getMarketTagName(stock.Market)
			if marketTag != "-" {
				tagMap[marketTag] = true
			}
		}
	}

	// 添加用户自定义标签
	for _, stock := range m.watchlist.Stocks {
		for _, tag := range stock.Tags {
			if tag != "" && tag != "-" {
				tagMap[tag] = true
			}
		}
	}

	tags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tags = append(tags, tag)
	}

	return tags
}

// ============================================================================
// WatchlistStock 标签方法
// ============================================================================

// hasTag 检查股票是否包含指定标签
func (stock *WatchlistStock) hasTag(tag string) bool {
	for _, t := range stock.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// addTag 添加标签到股票（避免重复）
func (stock *WatchlistStock) addTag(tag string) {
	if tag == "" || tag == "-" {
		return
	}
	if !stock.hasTag(tag) {
		stock.Tags = append(stock.Tags, tag)
	}
}

// removeTag 移除股票的标签
func (stock *WatchlistStock) removeTag(tag string) {
	for i, t := range stock.Tags {
		if t == tag {
			stock.Tags = append(stock.Tags[:i], stock.Tags[i+1:]...)
			break
		}
	}
}

// getTagsDisplay 获取股票标签的显示字符串（展示层动态组合市场标签和用户标签）
func (stock *WatchlistStock) getTagsDisplay(m *Model) string {
	// 从 market 字段生成市场标签
	marketTag := m.getMarketTagName(stock.Market)

	// 过滤用户自定义标签
	var validTags []string
	for _, tag := range stock.Tags {
		if tag != "" && tag != "-" {
			validTags = append(validTags, tag)
		}
	}

	// 组合市场标签 + 用户标签（市场标签优先）
	allTags := []string{marketTag}
	allTags = append(allTags, validTags...)

	// 如果只有市场标签且为 "-"，返回 "-"
	if len(allTags) == 1 && allTags[0] == "-" {
		return "-"
	}

	// 如果只有一个标签（市场标签）
	if len(allTags) == 1 {
		return allTags[0]
	}

	// 多个标签时，用逗号分隔，但如果太长则显示数量
	display := strings.Join(allTags, ",")
	totalLen := len(display)

	if totalLen > 15 {
		return fmt.Sprintf("%s+%d", allTags[0], len(allTags)-1)
	}

	return display
}

// ============================================================================
// 自选列表过滤和缓存
// ============================================================================

// getFilteredWatchlist 根据标签过滤自选股票（支持市场标签，带缓存优化）
func (m *Model) getFilteredWatchlist() []WatchlistStock {
	// 如果没有过滤标签，直接返回完整列表
	if m.selectedTag == "" {
		return m.watchlist.Stocks
	}

	// 检查缓存是否有效
	if m.isFilteredWatchlistValid && m.cachedFilterTag == m.selectedTag {
		return m.cachedFilteredWatchlist
	}

	// 重新计算过滤结果并缓存
	var filtered []WatchlistStock
	for _, stock := range m.watchlist.Stocks {
		// 检查是否匹配市场标签
		marketTag := m.getMarketTagName(stock.Market)
		if marketTag == m.selectedTag {
			filtered = append(filtered, stock)
			continue
		}

		// 检查用户自定义标签
		if stock.hasTag(m.selectedTag) {
			filtered = append(filtered, stock)
		}
	}

	// 更新缓存
	m.cachedFilteredWatchlist = filtered
	m.cachedFilterTag = m.selectedTag
	m.isFilteredWatchlistValid = true

	return filtered
}

// invalidateWatchlistCache 使缓存失效的辅助函数
func (m *Model) invalidateWatchlistCache() {
	m.isFilteredWatchlistValid = false
	m.cachedFilteredWatchlist = nil
	m.cachedFilterTag = ""
}

// ============================================================================
// 自选列表光标和滚动管理
// ============================================================================

// resetWatchlistCursor 重置自选列表游标到第一只股票（基于过滤后的列表）
func (m *Model) resetWatchlistCursor() {
	filteredStocks := m.getFilteredWatchlist()
	if len(filteredStocks) > 0 {
		m.watchlistCursor = 0
		maxWatchlistLines := m.config.Display.MaxLines
		if len(filteredStocks) > maxWatchlistLines {
			// 显示前N条：滚动位置设置为显示从索引0开始的N条
			m.watchlistScrollPos = len(filteredStocks) - maxWatchlistLines
		} else {
			// 股票数量不超过显示行数，显示全部
			m.watchlistScrollPos = 0
		}
	} else {
		// 没有股票时重置
		m.watchlistCursor = 0
		m.watchlistScrollPos = 0
	}
}

// adjustWatchlistScroll 调整自选列表滚动位置（基于过滤后的列表）
func (m *Model) adjustWatchlistScroll(filteredStocks []WatchlistStock) {
	maxWatchlistLines := m.config.Display.MaxLines
	totalStocks := len(filteredStocks)

	if totalStocks <= maxWatchlistLines {
		m.watchlistScrollPos = 0
		return
	}

	// 确保光标在可见范围内
	endIndex := totalStocks - m.watchlistScrollPos
	startIndex := endIndex - maxWatchlistLines
	if startIndex < 0 {
		startIndex = 0
	}

	// 如果光标超出可见范围的上边界，调整滚动位置
	if m.watchlistCursor < startIndex {
		m.watchlistScrollPos = totalStocks - m.watchlistCursor - maxWatchlistLines
		if m.watchlistScrollPos < 0 {
			m.watchlistScrollPos = 0
		}
	}

	// 如果光标超出可见范围的下边界，调整滚动位置
	if m.watchlistCursor >= endIndex {
		m.watchlistScrollPos = totalStocks - m.watchlistCursor - 1
		if m.watchlistScrollPos < 0 {
			m.watchlistScrollPos = 0
		}
	}
}

// ============================================================================
// 自选列表股票管理
// ============================================================================

// isStockInWatchlist 检查股票是否已在自选列表中
func (m *Model) isStockInWatchlist(code string) bool {
	for _, stock := range m.watchlist.Stocks {
		if stock.Code == code {
			return true
		}
	}
	return false
}

// addToWatchlist 添加股票到自选列表
func (m *Model) addToWatchlist(code, name string) bool {
	if m.isStockInWatchlist(code) {
		return false // 已在列表中
	}

	// 识别市场类型
	market := getMarketType(code)

	watchStock := WatchlistStock{
		Code:   code,
		Name:   name,
		Market: market,     // 保存市场类型
		Tags:   []string{}, // 初始为空，不包含市场标签
	}
	// 将新股票插入到列表首位，而不是末尾
	m.watchlist.Stocks = append([]WatchlistStock{watchStock}, m.watchlist.Stocks...)
	m.invalidateWatchlistCache() // 使缓存失效
	m.watchlistIsSorted = false  // 添加自选股票后重置自选列表排序状态
	m.saveWatchlist()
	return true
}

// removeFromWatchlist 从自选列表删除股票
func (m *Model) removeFromWatchlist(index int) {
	if index >= 0 && index < len(m.watchlist.Stocks) {
		m.watchlist.Stocks = append(m.watchlist.Stocks[:index], m.watchlist.Stocks[index+1:]...)
		m.invalidateWatchlistCache() // 使缓存失效
		m.saveWatchlist()
		m.watchlistIsSorted = false // 删除自选股票后重置自选列表排序状态
	}
}

// ============================================================================
// 标签相关状态处理器
// ============================================================================

// handleWatchlistTagging 处理自选股票打标签
func (m *Model) handleWatchlistTagging(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.tagInput == "" {
			// 回到标签管理界面
			m.availableTags = m.getAvailableTags()
			m.state = WatchlistTagManage
			m.tagManageCursor = 0
			return m, nil
		}

		// 更新当前选中股票的标签（基于过滤后的列表）
		filteredStocks := m.getFilteredWatchlist()
		if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
			stockToTag := filteredStocks[m.watchlistCursor]

			// 在原始列表中找到该股票并添加标签
			for i, stock := range m.watchlist.Stocks {
				if stock.Code == stockToTag.Code {
					// 处理多个标签（逗号分隔）
					newTags := strings.Split(m.tagInput, ",")
					for _, tag := range newTags {
						tag = strings.TrimSpace(tag)
						if tag != "" && tag != "-" {
							m.watchlist.Stocks[i].addTag(tag)
						}
					}
					// 如果没有有效标签，确保至少有默认标签
					if len(m.watchlist.Stocks[i].Tags) == 0 {
						m.watchlist.Stocks[i].Tags = []string{"-"}
					}

					// 更新当前股票标签列表
					m.currentStockTags = make([]string, 0)
					for _, tag := range m.watchlist.Stocks[i].Tags {
						if tag != "" && tag != "-" {
							m.currentStockTags = append(m.currentStockTags, tag)
						}
					}
					break
				}
			}

			m.invalidateWatchlistCache() // 使缓存失效
			m.saveWatchlist()

			if m.language == Chinese {
				m.message = fmt.Sprintf("已为 %s 添加标签: %s",
					stockToTag.Name, m.tagInput)
			} else {
				m.message = fmt.Sprintf("Added tags to %s: %s",
					stockToTag.Name, m.tagInput)
			}
		}

		// 回到标签管理界面，更新可用标签列表
		m.availableTags = m.getAvailableTags()
		m.state = WatchlistTagManage
		m.tagManageCursor = 0
		m.tagInput = ""
		m.tagInputCursor = 0
		return m, nil
	case "esc", "q":
		// 回到标签管理界面
		m.availableTags = m.getAvailableTags()
		m.state = WatchlistTagManage
		m.tagManageCursor = 0
		m.tagInput = ""
		m.tagInputCursor = 0
		m.message = ""
		return m, nil
	case "left", "ctrl+b":
		// 光标左移
		if m.tagInputCursor > 0 {
			m.tagInputCursor--
		}
		return m, nil
	case "right", "ctrl+f":
		// 光标右移
		runes := []rune(m.tagInput)
		if m.tagInputCursor < len(runes) {
			m.tagInputCursor++
		}
		return m, nil
	case "home", "ctrl+a":
		// 光标移到开头
		m.tagInputCursor = 0
		return m, nil
	case "end", "ctrl+e":
		// 光标移到末尾
		m.tagInputCursor = len([]rune(m.tagInput))
		return m, nil
	case "backspace":
		// 删除光标前的字符
		m.tagInput, m.tagInputCursor = deleteRuneBeforeCursor(m.tagInput, m.tagInputCursor)
		return m, nil
	case "delete", "ctrl+d":
		// 删除光标处的字符
		m.tagInput, m.tagInputCursor = deleteRuneAtCursor(m.tagInput, m.tagInputCursor)
		return m, nil
	default:
		// 处理文本输入
		str := msg.String()
		if len(str) > 0 && str != "\n" && str != "\r" && !isControlKey(str) {
			m.tagInput, m.tagInputCursor = insertStringAtCursor(m.tagInput, m.tagInputCursor, str)
		}
		return m, nil
	}
}

// handleWatchlistGroupSelect 处理自选股票分组选择
func (m *Model) handleWatchlistGroupSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.availableTags) {
			m.selectedTag = m.availableTags[m.cursor]
		}
		m.invalidateWatchlistCache() // 使缓存失效
		m.state = WatchlistViewing
		m.message = ""
		m.resetWatchlistCursor() // 重置游标到第一只股票（考虑过滤）
		return m, m.tickCmd()    // 重启定时器
	case "esc", "q":
		m.selectedTag = ""           // 清除过滤
		m.invalidateWatchlistCache() // 使缓存失效
		m.state = WatchlistViewing
		m.resetWatchlistCursor() // 重置游标到第一只股票
		m.message = ""
		return m, m.tickCmd() // 重启定时器
	case "c":
		// 清除过滤，显示所有股票
		m.selectedTag = ""
		m.invalidateWatchlistCache() // 使缓存失效
		m.state = WatchlistViewing
		m.resetWatchlistCursor() // 重置游标到第一只股票
		m.message = ""
		return m, m.tickCmd() // 重启定时器
	case "up", "k", "w":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j", "s":
		if m.cursor < len(m.availableTags)-1 {
			m.cursor++
		}
		return m, nil
	}
	return m, nil
}

// ============================================================================
// 标签相关视图函数
// ============================================================================

// viewWatchlistTagging 打标签视图
func (m *Model) viewWatchlistTagging() string {
	var s string

	if m.language == Chinese {
		s += "=== 设置标签 ===\n\n"
	} else {
		s += "=== Set Tag ===\n\n"
	}

	filteredStocks := m.getFilteredWatchlist()
	if m.watchlistCursor >= 0 && m.watchlistCursor < len(filteredStocks) {
		stock := filteredStocks[m.watchlistCursor]
		marketTag := m.getMarketTagName(stock.Market)

		if m.language == Chinese {
			s += fmt.Sprintf("股票: %s (%s)\n", stock.Name, stock.Code)
			s += fmt.Sprintf("%s: %s\n", m.getText("marketInfo"), marketTag)
			s += fmt.Sprintf("当前标签: %s\n\n", stock.getTagsDisplay(m))
			s += "请输入新标签(多个标签用逗号分隔): " + formatTextWithCursor(m.tagInput, m.tagInputCursor) + "\n\n"
			s += "操作: ←/→移动光标, Enter确认, ESC/Q取消, Home/End跳转首尾"
		} else {
			s += fmt.Sprintf("Stock: %s (%s)\n", stock.Name, stock.Code)
			s += fmt.Sprintf("%s: %s\n", m.getText("marketInfo"), marketTag)
			s += fmt.Sprintf("Current tags: %s\n\n", stock.getTagsDisplay(m))
			s += "Enter new tags (comma separated): " + formatTextWithCursor(m.tagInput, m.tagInputCursor) + "\n\n"
			s += "Actions: ←/→ move cursor, Enter confirm, ESC/Q cancel, Home/End jump"
		}
	}

	return s
}

// viewWatchlistGroupSelect 分组选择视图
func (m *Model) viewWatchlistGroupSelect() string {
	var s string

	if m.language == Chinese {
		s += "=== 选择标签分组 ===\n\n"
	} else {
		s += "=== Select Tag Group ===\n\n"
	}

	// 显示当前过滤状态
	if m.selectedTag != "" {
		if m.language == Chinese {
			s += fmt.Sprintf("当前过滤: %s\n\n", m.selectedTag)
		} else {
			s += fmt.Sprintf("Current filter: %s\n\n", m.selectedTag)
		}
	}

	// 显示可用标签列表
	if len(m.availableTags) == 0 {
		if m.language == Chinese {
			s += "暂无可用标签\n"
		} else {
			s += "No tags available\n"
		}
	} else {
		for i, tag := range m.availableTags {
			cursor := " "
			if i == m.cursor {
				cursor = ">"
			}
			s += fmt.Sprintf("%s %s\n", cursor, tag)
		}
	}

	s += "\n"
	if m.language == Chinese {
		s += "操作: ↑/↓选择, Enter确认, C清除过滤, ESC/Q返回"
	} else {
		s += "Actions: ↑/↓ select, Enter confirm, C clear filter, ESC/Q back"
	}

	return s
}
