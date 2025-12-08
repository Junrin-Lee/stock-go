package main

import (
	"fmt"
	"strings"
	"time"
)

// ============================================================================
// 调试日志系统
// ============================================================================

// globalModel 全局模型引用，用于调试日志记录
var globalModel *Model

// debugPrint 调试输出函数 - 支持 i18n key
// key 参数是 i18n 键名，如 "debug.api.directFail"
// args 是格式化参数，将替换翻译文本中的 %s, %d 等占位符
func debugPrint(key string, args ...any) {
	if globalModel != nil && globalModel.debugMode {
		timestamp := time.Now().Format("15:04:05")
		format := getDebugText(key)
		logMsg := fmt.Sprintf("[%s] %s", timestamp, fmt.Sprintf(format, args...))
		globalModel.addDebugLog(logMsg)
	}
}

// addDebugLog 添加调试日志
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

// logUserAction 记录用户操作 - 支持 i18n key
// actionKey 参数是 i18n 键名，如 "debug.action.enterPortfolio"
// args 是格式化参数，将替换翻译文本中的占位符
func (m *Model) logUserAction(actionKey string, args ...any) {
	if m.debugMode {
		timestamp := time.Now().Format("15:04:05")
		prefix := m.getText("debug.action.prefix")
		action := fmt.Sprintf(m.getText(actionKey), args...)
		logMsg := fmt.Sprintf("[%s] %s %s", timestamp, prefix, action)
		m.addDebugLog(logMsg)
	}
}

// ============================================================================
// 调试日志滚动控制
// ============================================================================

// scrollDebugUp 向上滚动调试日志
func (m *Model) scrollDebugUp() {
	maxScroll := len(m.debugLogs) - 1
	if m.debugScrollPos < maxScroll {
		m.debugScrollPos++
	}
}

// scrollDebugDown 向下滚动调试日志
func (m *Model) scrollDebugDown() {
	if m.debugScrollPos > 0 {
		m.debugScrollPos--
	}
}

// scrollDebugToTop 跳转到调试日志顶部
func (m *Model) scrollDebugToTop() {
	if len(m.debugLogs) > 0 {
		m.debugScrollPos = len(m.debugLogs) - 1
	}
}

// scrollDebugToBottom 跳转到调试日志底部
func (m *Model) scrollDebugToBottom() {
	m.debugScrollPos = 0
}

// ============================================================================
// 调试面板渲染
// ============================================================================

// renderDebugPanel 渲染调试面板
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
