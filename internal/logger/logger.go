package logger

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	globalLogger *Logger
	once         sync.Once
)

// Logger 日志记录器
type Logger struct {
	*logrus.Logger
	fields logrus.Fields
	mutex  sync.RWMutex
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `yaml:"level"`       // 日志级别
	Format     string `yaml:"format"`      // 日志格式 json/text
	Output     string `yaml:"output"`      // 输出 stdout/stderr/file
	FilePath   string `yaml:"file_path"`   // 文件路径
	MaxSize    int    `yaml:"max_size"`    // 最大文件大小(MB)
	MaxBackups int    `yaml:"max_backups"` // 最大备份数
	MaxAge     int    `yaml:"max_age"`     // 最大保存天数
}

// NewLogger 创建新的日志记录器
func NewLogger(config *LogConfig) *Logger {
	logger := &Logger{
		Logger: logrus.New(),
		fields: make(logrus.Fields),
	}

	// 设置日志级别
	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// 设置日志格式
	if config.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	// 设置输出
	switch config.Output {
	case "stderr":
		logger.SetOutput(os.Stderr)
	case "file":
		if config.FilePath != "" {
			file, err := os.OpenFile(config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				logger.SetOutput(file)
			} else {
				logger.SetOutput(os.Stdout)
				logger.Warn("Failed to open log file, using stdout")
			}
		}
	default:
		logger.SetOutput(os.Stdout)
	}

	// 添加钩子
	logger.AddHook(&CallerHook{})

	return logger
}

// WithField 添加字段
func (l *Logger) WithField(key string, value interface{}) *Logger {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	newLogger := &Logger{
		Logger: l.Logger,
		fields: make(logrus.Fields),
	}

	// 复制现有字段
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// 添加新字段
	newLogger.fields[key] = value

	return newLogger
}

// WithFields 添加多个字段
func (l *Logger) WithFields(fields logrus.Fields) *Logger {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	newLogger := &Logger{
		Logger: l.Logger,
		fields: make(logrus.Fields),
	}

	// 复制现有字段
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// 添加新字段
	for k, v := range fields {
		newLogger.fields[k] = v
	}

	return newLogger
}

// GetEntry 获取日志条目
func (l *Logger) getEntry() *logrus.Entry {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	return l.Logger.WithFields(l.fields)
}

// Debug 调试日志
func (l *Logger) Debug(args ...interface{}) {
	l.getEntry().Debug(args...)
}

// Debugf 格式化调试日志
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.getEntry().Debugf(format, args...)
}

// Info 信息日志
func (l *Logger) Info(args ...interface{}) {
	l.getEntry().Info(args...)
}

// Infof 格式化信息日志
func (l *Logger) Infof(format string, args ...interface{}) {
	l.getEntry().Infof(format, args...)
}

// Warn 警告日志
func (l *Logger) Warn(args ...interface{}) {
	l.getEntry().Warn(args...)
}

// Warnf 格式化警告日志
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.getEntry().Warnf(format, args...)
}

// Error 错误日志
func (l *Logger) Error(args ...interface{}) {
	l.getEntry().Error(args...)
}

// Errorf 格式化错误日志
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.getEntry().Errorf(format, args...)
}

// Fatal 致命错误日志
func (l *Logger) Fatal(args ...interface{}) {
	l.getEntry().Fatal(args...)
}

// Fatalf 格式化致命错误日志
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.getEntry().Fatalf(format, args...)
}

// Panic 恐慌日志
func (l *Logger) Panic(args ...interface{}) {
	l.getEntry().Panic(args...)
}

// Panicf 格式化恐慌日志
func (l *Logger) Panicf(format string, args ...interface{}) {
	l.getEntry().Panicf(format, args...)
}

// CallerHook 调用者钩子，添加文件名和行号信息
type CallerHook struct{}

// Levels 支持的日志级别
func (hook *CallerHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire 执行钩子
func (hook *CallerHook) Fire(entry *logrus.Entry) error {
	// 获取调用者信息
	if pc, file, line, ok := runtime.Caller(8); ok {
		funcName := runtime.FuncForPC(pc).Name()
		entry.Data["caller"] = fmt.Sprintf("%s:%d", file, line)
		entry.Data["func"] = funcName
	}
	return nil
}

// InitGlobalLogger 初始化全局日志记录器
func InitGlobalLogger(config *LogConfig) {
	once.Do(func() {
		globalLogger = NewLogger(config)
	})
}

// GetGlobalLogger 获取全局日志记录器
func GetGlobalLogger() *Logger {
	if globalLogger == nil {
		// 使用默认配置初始化
		InitGlobalLogger(&LogConfig{
			Level:  "info",
			Format: "text",
			Output: "stdout",
		})
	}
	return globalLogger
}

// 全局日志函数
func Debug(args ...interface{}) {
	GetGlobalLogger().Debug(args...)
}

func Debugf(format string, args ...interface{}) {
	GetGlobalLogger().Debugf(format, args...)
}

func Info(args ...interface{}) {
	GetGlobalLogger().Info(args...)
}

func Infof(format string, args ...interface{}) {
	GetGlobalLogger().Infof(format, args...)
}

func Warn(args ...interface{}) {
	GetGlobalLogger().Warn(args...)
}

func Warnf(format string, args ...interface{}) {
	GetGlobalLogger().Warnf(format, args...)
}

func Error(args ...interface{}) {
	GetGlobalLogger().Error(args...)
}

func Errorf(format string, args ...interface{}) {
	GetGlobalLogger().Errorf(format, args...)
}

func Fatal(args ...interface{}) {
	GetGlobalLogger().Fatal(args...)
}

func Fatalf(format string, args ...interface{}) {
	GetGlobalLogger().Fatalf(format, args...)
}

func Panic(args ...interface{}) {
	GetGlobalLogger().Panic(args...)
}

func Panicf(format string, args ...interface{}) {
	GetGlobalLogger().Panicf(format, args...)
}

func WithField(key string, value interface{}) *Logger {
	return GetGlobalLogger().WithField(key, value)
}

func WithFields(fields logrus.Fields) *Logger {
	return GetGlobalLogger().WithFields(fields)
}

// LogWriter 日志写入器，实现io.Writer接口
type LogWriter struct {
	logger *Logger
	level  logrus.Level
}

// NewLogWriter 创建日志写入器
func NewLogWriter(logger *Logger, level logrus.Level) *LogWriter {
	return &LogWriter{
		logger: logger,
		level:  level,
	}
}

// Write 写入日志
func (w *LogWriter) Write(p []byte) (n int, err error) {
	switch w.level {
	case logrus.DebugLevel:
		w.logger.Debug(string(p))
	case logrus.InfoLevel:
		w.logger.Info(string(p))
	case logrus.WarnLevel:
		w.logger.Warn(string(p))
	case logrus.ErrorLevel:
		w.logger.Error(string(p))
	case logrus.FatalLevel:
		w.logger.Fatal(string(p))
	case logrus.PanicLevel:
		w.logger.Panic(string(p))
	default:
		w.logger.Info(string(p))
	}
	return len(p), nil
}

// MultiWriter 多重写入器
type MultiWriter struct {
	writers []io.Writer
}

// NewMultiWriter 创建多重写入器
func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	return &MultiWriter{writers: writers}
}

// Write 写入到所有写入器
func (mw *MultiWriter) Write(p []byte) (n int, err error) {
	for _, w := range mw.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
	}
	return len(p), nil
}

// PerformanceLogger 性能日志记录器
type PerformanceLogger struct {
	logger    *Logger
	startTime time.Time
	operation string
}

// NewPerformanceLogger 创建性能日志记录器
func NewPerformanceLogger(operation string) *PerformanceLogger {
	return &PerformanceLogger{
		logger:    GetGlobalLogger(),
		startTime: time.Now(),
		operation: operation,
	}
}

// End 结束性能测量
func (p *PerformanceLogger) End() {
	duration := time.Since(p.startTime)
	p.logger.WithFields(logrus.Fields{
		"operation":   p.operation,
		"duration":    duration.String(),
		"duration_ms": duration.Milliseconds(),
	}).Info("Performance measurement")
}

// EndWithFields 带字段结束性能测量
func (p *PerformanceLogger) EndWithFields(fields logrus.Fields) {
	duration := time.Since(p.startTime)
	fields["operation"] = p.operation
	fields["duration"] = duration.String()
	fields["duration_ms"] = duration.Milliseconds()

	p.logger.WithFields(fields).Info("Performance measurement")
}
