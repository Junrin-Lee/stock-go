package main

import "fmt"

// ============================================================================
// 日志函数 - 四个级别
// ============================================================================

// logDebug DEBUG 级别日志 - 详细调试信息
// key: i18n 键名（如 "log.api.requestDetail"）
// args: 格式化参数（替换 i18n 文本中的 %s, %d 等占位符）
func logDebug(key string, args ...any) {
	if globalLogger == nil {
		return
	}

	// 获取 i18n 文本
	text := getLogText(key)
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}

	// 写入日志文件
	globalLogger.Log(LogDebug, key, text)
}

// logInfo INFO 级别日志 - 正常运行信息
// key: i18n 键名（如 "log.intraday.workerStart"）
// args: 格式化参数
func logInfo(key string, args ...any) {
	if globalLogger == nil {
		return
	}

	text := getLogText(key)
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}

	globalLogger.Log(LogInfo, key, text)
}

// logWarn WARN 级别日志 - 可能的问题
// key: i18n 键名（如 "log.api.fallback"）
// args: 格式化参数
func logWarn(key string, args ...any) {
	if globalLogger == nil {
		return
	}

	text := getLogText(key)
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}

	globalLogger.Log(LogWarn, key, text)
}

// logError ERROR 级别日志 - 需要关注的错误
// key: i18n 键名（如 "log.api.allFailed"）
// args: 格式化参数
func logError(key string, args ...any) {
	if globalLogger == nil {
		return
	}

	text := getLogText(key)
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}

	globalLogger.Log(LogError, key, text)
}

// ============================================================================
// 辅助函数
// ============================================================================

// getLogText 获取 i18n 日志文本
// key: i18n 键名
// 返回: 翻译后的文本，如果找不到则返回 key 本身
func getLogText(key string) string {
	if globalModel != nil {
		return globalModel.getText(key)
	}
	// 如果 globalModel 未初始化，返回 key 作为后备
	return key
}

// ============================================================================
// 简化日志函数 - 用于没有 i18n key 的直接消息
// 格式: [time][level][message]
// ============================================================================

// logInfoDirect 直接记录 INFO 级别消息（无 key）
func logInfoDirect(format string, args ...any) {
	if globalLogger == nil {
		return
	}
	message := fmt.Sprintf(format, args...)
	globalLogger.Log(LogInfo, "", message)
}

// logDebugDirect 直接记录 DEBUG 级别消息（无 key）
func logDebugDirect(format string, args ...any) {
	if globalLogger == nil {
		return
	}
	message := fmt.Sprintf(format, args...)
	globalLogger.Log(LogDebug, "", message)
}

// logWarnDirect 直接记录 WARN 级别消息（无 key）
func logWarnDirect(format string, args ...any) {
	if globalLogger == nil {
		return
	}
	message := fmt.Sprintf(format, args...)
	globalLogger.Log(LogWarn, "", message)
}

// logErrorDirect 直接记录 ERROR 级别消息（无 key）
func logErrorDirect(format string, args ...any) {
	if globalLogger == nil {
		return
	}
	message := fmt.Sprintf(format, args...)
	globalLogger.Log(LogError, "", message)
}
