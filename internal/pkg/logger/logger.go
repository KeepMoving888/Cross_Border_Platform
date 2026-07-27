package logger

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.SugaredLogger

// Init 初始化日志
func Init(level, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create log dir failed: %w", err)
	}

	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	consoleEncoder := zapcore.NewConsoleEncoder(encoderCfg)
	jsonEncoder := zapcore.NewJSONEncoder(encoderCfg)

	infoFile := filepath.Join(dir, "app.log")
	errorFile := filepath.Join(dir, "error.log")

	infoWriter, err := getLogWriter(infoFile)
	if err != nil {
		return err
	}
	errorWriter, err := getLogWriter(errorFile)
	if err != nil {
		return err
	}

	consoleSyncer := zapcore.AddSync(os.Stdout)

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, consoleSyncer, zapLevel),
		zapcore.NewCore(jsonEncoder, infoWriter, zapLevel),
		zapcore.NewCore(jsonEncoder, errorWriter, zapcore.ErrorLevel),
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0), zap.AddStacktrace(zapcore.ErrorLevel))
	log = logger.Sugar()
	return nil
}

func getLogWriter(path string) (zapcore.WriteSyncer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s failed: %w", path, err)
	}
	return zapcore.AddSync(f), nil
}

// Get 获取日志实例
func Get() *zap.SugaredLogger {
	if log == nil {
		// 兜底:返回开发模式日志
		l, _ := zap.NewDevelopment()
		log = l.Sugar()
	}
	return log
}

// Sync 刷新日志缓冲
func Sync() {
	if log != nil {
		_ = log.Sync()
	}
}

// 便捷方法
func Debug(args ...interface{}) { Get().Debug(args...) }
func Info(args ...interface{})  { Get().Info(args...) }
func Warn(args ...interface{})  { Get().Warn(args...) }
func Error(args ...interface{}) { Get().Error(args...) }
func Fatal(args ...interface{}) { Get().Fatal(args...) }

func Debugf(format string, args ...interface{}) { Get().Debugf(format, args...) }
func Infof(format string, args ...interface{})  { Get().Infof(format, args...) }
func Warnf(format string, args ...interface{})  { Get().Warnf(format, args...) }
func Errorf(format string, args ...interface{}) { Get().Errorf(format, args...) }
func Fatalf(format string, args ...interface{}) { Get().Fatalf(format, args...) }

func With(fields ...interface{}) *zap.SugaredLogger { return Get().With(fields...) }
