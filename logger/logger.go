package logger

import (
	"fmt"
	"log"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	INFO LogLevel = iota
	DEBUG
)

var (
	currentLevel LogLevel = INFO
	timeFormat   string   = "2006-01-02 15:04:05"
)

// SetLevel 设置日志级别
func SetLevel(level LogLevel) {
	currentLevel = level
}

// SetLevelFromString 从字符串设置日志级别
func SetLevelFromString(level string) {
	switch level {
	case "debug", "DEBUG":
		currentLevel = DEBUG
	case "info", "INFO":
		currentLevel = INFO
	default:
		currentLevel = INFO
	}
}

// GetLevelString 获取当前日志级别字符串
func GetLevelString() string {
	switch currentLevel {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	default:
		return "INFO"
	}
}

// Info 输出INFO级别日志
func Info(format string, args ...interface{}) {
	if currentLevel >= INFO {
		log.Printf("[INFO] "+format, args...)
	}
}

// Debug 输出DEBUG级别日志
func Debug(format string, args ...interface{}) {
	if currentLevel >= DEBUG {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Error 输出错误日志（总是显示）
func Error(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// InfoWithTime 带时间戳的INFO日志
func InfoWithTime(format string, args ...interface{}) {
	if currentLevel >= INFO {
		timestamp := time.Now().Format(timeFormat)
		log.Printf("[INFO][%s] "+format, append([]interface{}{timestamp}, args...)...)
	}
}

// DebugWithTime 带时间戳的DEBUG日志
func DebugWithTime(format string, args ...interface{}) {
	if currentLevel >= DEBUG {
		timestamp := time.Now().Format(timeFormat)
		log.Printf("[DEBUG][%s] "+format, append([]interface{}{timestamp}, args...)...)
	}
}

// NetworkDiag 网络诊断专用日志（仅DEBUG级别）
func NetworkDiag(format string, args ...interface{}) {
	if currentLevel >= DEBUG {
		log.Printf("[DEBUG][NETWORK] "+format, args...)
	}
}

// ConnectionDiag 连接诊断专用日志（仅DEBUG级别）
func ConnectionDiag(format string, args ...interface{}) {
	if currentLevel >= DEBUG {
		log.Printf("[DEBUG][CONN] "+format, args...)
	}
}

// GRPCDiag gRPC诊断专用日志（仅DEBUG级别）
func GRPCDiag(format string, args ...interface{}) {
	if currentLevel >= DEBUG {
		log.Printf("[DEBUG][GRPC] "+format, args...)
	}
}

// 简化的日志函数别名（向后兼容）
func Printf(format string, args ...interface{}) {
	Info(format, args...)
}

func Println(args ...interface{}) {
	Info(fmt.Sprint(args...))
}
