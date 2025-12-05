package main

import (
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ============================================================================
// 文本编辑辅助函数
// ============================================================================

// insertRuneAtCursor 在光标位置插入字符
func insertRuneAtCursor(text string, cursor int, r rune) (string, int) {
	runes := []rune(text)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	// 在光标位置插入字符
	newRunes := make([]rune, len(runes)+1)
	copy(newRunes[:cursor], runes[:cursor])
	newRunes[cursor] = r
	copy(newRunes[cursor+1:], runes[cursor:])

	return string(newRunes), cursor + 1
}

// insertStringAtCursor 在光标位置插入字符串
func insertStringAtCursor(text string, cursor int, insert string) (string, int) {
	runes := []rune(text)
	insertRunes := []rune(insert)

	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	// 在光标位置插入字符串
	newRunes := make([]rune, len(runes)+len(insertRunes))
	copy(newRunes[:cursor], runes[:cursor])
	copy(newRunes[cursor:cursor+len(insertRunes)], insertRunes)
	copy(newRunes[cursor+len(insertRunes):], runes[cursor:])

	return string(newRunes), cursor + len(insertRunes)
}

// deleteRuneBeforeCursor 删除光标前的字符（退格键）
func deleteRuneBeforeCursor(text string, cursor int) (string, int) {
	runes := []rune(text)
	if cursor <= 0 || len(runes) == 0 {
		return text, cursor
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	// 删除光标前的字符
	newRunes := make([]rune, len(runes)-1)
	copy(newRunes[:cursor-1], runes[:cursor-1])
	copy(newRunes[cursor-1:], runes[cursor:])

	return string(newRunes), cursor - 1
}

// deleteRuneAtCursor 删除光标处的字符（Delete键）
func deleteRuneAtCursor(text string, cursor int) (string, int) {
	runes := []rune(text)
	if cursor < 0 || cursor >= len(runes) || len(runes) == 0 {
		return text, cursor
	}

	// 删除光标处的字符
	newRunes := make([]rune, len(runes)-1)
	copy(newRunes[:cursor], runes[:cursor])
	copy(newRunes[cursor:], runes[cursor+1:])

	return string(newRunes), cursor
}

// formatTextWithCursor 格式化带光标的文本用于显示
func formatTextWithCursor(text string, cursor int) string {
	runes := []rune(text)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	// 在光标位置插入光标符号
	if cursor == len(runes) {
		return text + "│"
	}

	before := string(runes[:cursor])
	after := string(runes[cursor:])
	return before + "│" + after
}

// handleTextInput 通用输入处理函数：处理光标移动和文本编辑
func handleTextInput(msg tea.KeyMsg, text *string, cursor *int) bool {
	switch msg.String() {
	case "left", "ctrl+b":
		if *cursor > 0 {
			*cursor--
		}
		return true
	case "right", "ctrl+f":
		runes := []rune(*text)
		if *cursor < len(runes) {
			*cursor++
		}
		return true
	case "home", "ctrl+a":
		*cursor = 0
		return true
	case "end", "ctrl+e":
		*cursor = len([]rune(*text))
		return true
	case "backspace":
		*text, *cursor = deleteRuneBeforeCursor(*text, *cursor)
		return true
	case "delete", "ctrl+d":
		*text, *cursor = deleteRuneAtCursor(*text, *cursor)
		return true
	default:
		str := msg.String()
		if len(str) > 0 && str != "\n" && str != "\r" && !isControlKey(str) {
			*text, *cursor = insertStringAtCursor(*text, *cursor, str)
			return true
		}
	}
	return false
}

// ============================================================================
// 控制键检测
// ============================================================================

// isControlKey 检查是否为控制键
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
		"insert", "delete", "tab", "enter", "backspace", "esc",
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

// ============================================================================
// 字符编码转换
// ============================================================================

// gbkToUtf8 将GBK编码转换为UTF-8
func gbkToUtf8(data []byte) (string, error) {
	reader := transform.NewReader(strings.NewReader(string(data)), simplifiedchinese.GBK.NewDecoder())
	utf8Data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(utf8Data), nil
}
