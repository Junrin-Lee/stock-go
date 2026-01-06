package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ============================================================================
// 日志级别定义
// ============================================================================

// LogLevel 日志级别
type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

// ============================================================================
// Logger 结构
// ============================================================================

// Logger 封装 zap logger，支持按天自动轮转
type Logger struct {
	mu         sync.Mutex  // 保护 zap 实例和 currentDay 的并发访问
	zap        *zap.Logger // zap logger 实例
	currentDay string      // 当前日志文件对应的日期 (YYYY-MM-DD)
	logDir     string      // 日志目录路径
	level      LogLevel    // 最低日志级别
}

var globalLogger *Logger

// ============================================================================
// 初始化
// ============================================================================

// InitLogger 初始化全局日志系统
func InitLogger(logDir string, level LogLevel) error {
	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	globalLogger = &Logger{
		logDir: logDir,
		level:  level,
	}

	// 立即轮转到今天的日志文件
	return globalLogger.rotateIfNeeded()
}

// ============================================================================
// 日志轮转
// ============================================================================

// rotateIfNeeded 检查是否需要切换到新的日志文件（按天轮转）
func (l *Logger) rotateIfNeeded() error {
	today := time.Now().Format("2006-01-02")

	// 快速路径：如果是同一天且 logger 已存在，无需轮转
	if l.currentDay == today && l.zap != nil {
		return nil
	}

	// 双重检查锁（DCL）：防止并发轮转
	l.mu.Lock()
	defer l.mu.Unlock()

	// 再次检查（可能其他 goroutine 已完成轮转）
	if l.currentDay == today && l.zap != nil {
		return nil
	}

	// 关闭旧 logger（刷新缓冲区）
	if l.zap != nil {
		l.zap.Sync()
	}

	// 创建新日志文件
	logPath := filepath.Join(l.logDir, fmt.Sprintf("stock-monitor-%s.log", today))
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// 配置编码器（自定义格式）
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:    "time",
		LevelKey:   "level",
		MessageKey: "msg",
		LineEnding: zapcore.DefaultLineEnding,
		// 自定义编码器：[2006-01-02 15:04:05][DEBUG]
		EncodeLevel: bracketLevelEncoder,
		EncodeTime:  bracketTimeEncoder,
	}

	// 构建 zapcore
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(file),
		levelToZapLevel(l.level),
	)

	// 创建新 logger
	l.zap = zap.New(core)
	l.currentDay = today

	return nil
}

// ============================================================================
// 编码器
// ============================================================================

// bracketTimeEncoder 自定义时间编码器: [2006-01-02 15:04:05]
func bracketTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString("[" + t.Format("2006-01-02 15:04:05") + "]")
}

// bracketLevelEncoder 自定义级别编码器: [DEBUG]
func bracketLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString("[" + l.CapitalString() + "]")
}

// levelToZapLevel 将自定义 LogLevel 转换为 zapcore.Level
func levelToZapLevel(l LogLevel) zapcore.Level {
	switch l {
	case LogDebug:
		return zapcore.DebugLevel
	case LogInfo:
		return zapcore.InfoLevel
	case LogWarn:
		return zapcore.WarnLevel
	case LogError:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// ============================================================================
// 日志接口
// ============================================================================

// Log 统一日志接口
// level: 日志级别
// pathKey: i18n 键名（作为日志标识符，便于过滤），为空时不输出
// message: 格式化后的日志消息
func (l *Logger) Log(level LogLevel, pathKey string, message string) {
	// 每次写日志前检查是否需要轮转（跨天时自动切换文件）
	l.rotateIfNeeded()

	// 格式: [pathKey][message] 或 [message]（如果 pathKey 为空）
	var formatted string
	if pathKey != "" {
		formatted = "[" + pathKey + "][" + message + "]"
	} else {
		formatted = "[" + message + "]"
	}

	switch level {
	case LogDebug:
		l.zap.Debug(formatted)
	case LogInfo:
		l.zap.Info(formatted)
	case LogWarn:
		l.zap.Warn(formatted)
	case LogError:
		l.zap.Error(formatted)
	}
}

// Sync 刷新缓冲区（应用退出时调用）
func (l *Logger) Sync() {
	if l.zap != nil {
		l.zap.Sync()
	}
}
