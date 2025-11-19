package main

import (
	"strings"

	"github.com/jedib0t/go-pretty/v6/text"
)

// ColorUtils 颜色工具类
type ColorUtils struct{}

// NewColorUtils 创建颜色工具实例
func NewColorUtils() *ColorUtils {
	return &ColorUtils{}
}

// GetSupportedColors 获取go-pretty支持的所有颜色
func (c *ColorUtils) GetSupportedColors() map[string]text.Color {
	return map[string]text.Color{
		// 基本前景色
		"black":   text.FgBlack,
		"red":     text.FgRed,
		"green":   text.FgGreen,
		"yellow":  text.FgYellow,
		"blue":    text.FgBlue,
		"magenta": text.FgMagenta,
		"cyan":    text.FgCyan,
		"white":   text.FgWhite,
	}
}

// FormatTextWithBackground 使用指定背景色格式化文本
func (c *ColorUtils) FormatTextWithBackground(textContent, colorName string) string {
	colors := c.GetSupportedColors()
	color, exists := colors[strings.ToLower(colorName)]
	
	if !exists {
		color = text.FgYellow // 默认黄色前景色
	}
	
	return color.Sprint(textContent)
}

// GetColorFromConfigOrDefault 从配置获取颜色，如果无效则使用默认背景色
func (c *ColorUtils) GetColorFromConfigOrDefault(configColor, defaultColor string) string {
	if configColor == "" {
		return defaultColor
	}
	
	colors := c.GetSupportedColors()
	if _, exists := colors[strings.ToLower(configColor)]; exists {
		return configColor
	}
	
	return defaultColor
}