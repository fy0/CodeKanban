package utils

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerContextKey struct{}

var (
	globalLogger *zap.Logger
	loggerKey    = loggerContextKey{}
)

// InitLogger 会按配置初始化 zap 日志器，并返回日志实例与清理函数。
func InitLogger(cfg *AppConfig) (*zap.Logger, func(), error) {
	level := cfg.EffectiveLogLevel()

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Local().Format("2006-01-02 15:04:05.000"))
	}
	encoderCfg.TimeKey = "timestamp"

	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.AddSync(os.Stdout),
		zapcore.Level(level),
	)

	cores := []zapcore.Core{consoleCore}

	var logFile *os.File
	if cfg.LogFile != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755); err != nil {
			return nil, nil, err
		}

		file, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, err
		}
		logFile = file

		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderCfg),
			zapcore.AddSync(file),
			zapcore.Level(level),
		)
		cores = append(cores, fileCore)
	}

	logger := zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	globalLogger = logger

	cleanup := func() {
		_ = logger.Sync()
		if logFile != nil {
			_ = logFile.Close()
		}
	}

	return logger, cleanup, nil
}

// Logger 返回当前全局日志器，若未初始化则构造一个开发模式日志器。
func Logger() *zap.Logger {
	if globalLogger != nil {
		return globalLogger
	}
	logger, _ := zap.NewDevelopment()
	globalLogger = logger
	return logger
}

// ContextWithLogger 将日志器写入上下文，方便在业务层级传递。
func ContextWithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	if logger == nil {
		logger = Logger()
	}
	return context.WithValue(ctx, loggerKey, logger)
}

// LoggerFromContext 从上下文读取日志器，若不存在则返回全局日志器。
func LoggerFromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return Logger()
	}
	if logger, ok := ctx.Value(loggerKey).(*zap.Logger); ok && logger != nil {
		return logger
	}
	return Logger()
}

// LogLevel 提供统一的日志等级配置，便于未来扩展。
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// EffectiveLogLevel 会将配置中的字符串等级转换为 zapcore.Level。
func (c *AppConfig) EffectiveLogLevel() zapcore.Level {
	switch LogLevel(c.LogLevel) {
	case LogLevelDebug:
		return zapcore.DebugLevel
	case LogLevelWarn:
		return zapcore.WarnLevel
	case LogLevelError:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
