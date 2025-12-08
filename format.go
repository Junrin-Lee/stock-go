package main

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/text"
)

// ============================================================================
// 盈亏格式化函数 - 支持多语言颜色方案
// 中文：红涨绿跌 | 英文：绿涨红跌
// ============================================================================

// formatProfitWithColorLang 格式化盈亏金额（带颜色）
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

// formatProfitRateWithColorLang 格式化盈亏比例（带颜色）
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

// formatProfitWithColorZeroLang 格式化盈亏金额（零值显示白色）
func (m *Model) formatProfitWithColorZeroLang(profit float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(profit) < 0.001 {
		return fmt.Sprintf("%.2f", profit)
	}
	// 否则使用语言相关颜色逻辑
	return m.formatProfitWithColorLang(profit)
}

// formatProfitRateWithColorZeroLang 格式化盈亏比例（零值显示白色）
func (m *Model) formatProfitRateWithColorZeroLang(rate float64) string {
	// 当数值接近0时（考虑浮点数精度），显示白色（无颜色）
	if abs(rate) < 0.001 {
		return fmt.Sprintf("%.2f%%", rate)
	}
	// 否则使用语言相关颜色逻辑
	return m.formatProfitRateWithColorLang(rate)
}

// formatProfitRateWithColorZeroLangForStock 格式化盈亏率（支持股票类型检测）
func (m *Model) formatProfitRateWithColorZeroLangForStock(rate float64, symbol string) string {
	// 对于非A股（如美股），显示 "-" 表示数据不可用
	if !isChinaStock(symbol) {
		return "-"
	}
	return m.formatProfitRateWithColorZeroLang(rate)
}

// formatPriceWithColorLang 格式化价格（根据涨跌显示颜色）
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

// ============================================================================
// 其他格式化函数
// ============================================================================

// formatVolume 格式化成交量（万/亿）
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

// formatStockNameWithPortfolioHighlight 格式化股票名称（持仓股票高亮）
func (m *Model) formatStockNameWithPortfolioHighlight(name, code string) string {
	if m.isStockInPortfolio(code) {
		// 使用颜色工具处理背景高亮
		colorUtils := NewColorUtils()
		configColor := m.config.Display.PortfolioHighlight

		debugPrint("debug.highlight.found", name, code, configColor)

		// 获取最终的颜色名称（仅支持go-pretty颜色名称）
		finalColorName := colorUtils.GetColorFromConfigOrDefault(configColor, "yellow") // 默认黄色背景

		debugPrint("debug.highlight.finalColor", finalColorName)

		// 应用背景颜色格式化
		result := colorUtils.FormatTextWithBackground(name, finalColorName)
		debugPrint("debug.highlight.result", result, name)

		return result
	}
	return name
}

// ============================================================================
// 辅助函数
// ============================================================================

// abs 返回浮点数的绝对值
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
