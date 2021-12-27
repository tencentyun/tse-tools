package log

import (
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
)

type logOptions struct {
	encoderConfig zapcore.EncoderConfig
	loggers       []io.Writer
	level         zapcore.Level

	// options about zap.logger
	enableCaller      bool // whether show line number.
	enableDevelopment bool // whether show stacktrace when error

	// options about zapcore.core
	enableStdout        bool // whether send to stdout
	enableDefaultOutput bool // whether send to default output
}

type Option interface {
	apply(options *logOptions)
}

type optionFunc func(opts *logOptions)

func (f optionFunc) apply(opts *logOptions) {
	f(opts)
}

// if enableCaller on, log will contain line and stacktrace(if panic/error),
// default is true.
func EnableCaller(on bool) Option {
	return optionFunc(func(options *logOptions) {
		options.enableCaller = on
	})
}

// if development on, log will contain stack when level is ERROR,
// default is false.
func EnableDevelopment(on bool) Option {
	return optionFunc(func(options *logOptions) {
		options.enableDevelopment = on
	})
}

// setting log level, default is debug.
func LevelOpt(l Level) Option {
	zapLevel, _ := l.toZapLevel()
	return optionFunc(func(options *logOptions) {
		options.level = zapLevel
	})
}

func AppendWriter(writers ...io.Writer) Option {
	return optionFunc(func(options *logOptions) {
		options.loggers = append(options.loggers, writers...)
	})
}

// whether send to stdout, default is true.
func EnableStdout(on bool) Option {
	return optionFunc(func(options *logOptions) {
		options.enableStdout = on
	})
}

// whether send to defaultLogFilePath, default is true.
func EnableDefaultOutput(on bool) Option {
	return optionFunc(func(options *logOptions) {
		options.enableDefaultOutput = on
	})
}

func CountableIOWriter(path string, size int, backupNum int, compress bool) io.Writer {
	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    size,
		MaxBackups: backupNum,
		MaxAge:     defaultLogMaxAge,
		Compress:   compress,
		LocalTime:  true,
	}
}
