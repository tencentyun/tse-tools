package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
)

func InitGlobalLogger(options ...Option) {
	logOpt, optionsZ := buildOpt(options...)
	core := newZapCore(*logOpt)
	loggerZ.zapLogger = zap.New(core, optionsZ...)
}

func newDefaultLogOptions() *logOptions {
	return &logOptions{
		encoderConfig: zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "loggers",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		loggers: []io.Writer{&lumberjack.Logger{
			Filename:   defaultLogFilePath,
			MaxSize:    defaultLogMaxSize,
			MaxBackups: defaultLogMaxBackups,
			MaxAge:     defaultLogMaxAge,
			Compress:   false,
			LocalTime:  true,
		}},
		level:               zapcore.InfoLevel,
		enableCaller:        true,
		enableDevelopment:   false,
		enableStdout:        false,
		enableDefaultOutput: true,
	}
}

func newZapCore(opt logOptions) zapcore.Core {
	if opt.enableStdout {
		opt.loggers = append(opt.loggers, os.Stdout)
	}
	var writerSync []zapcore.WriteSyncer
	for index, writer := range opt.loggers {
		if index == 0 && !opt.enableDefaultOutput {
			continue
		}
		writerSync = append(writerSync, zapcore.AddSync(writer))
	}

	return zapcore.NewCore(
		zapcore.NewConsoleEncoder(opt.encoderConfig),
		zapcore.NewMultiWriteSyncer(writerSync...),
		zap.NewAtomicLevelAt(opt.level),
	)
}

func buildOpt(options ...Option) (*logOptions, []zap.Option) {
	logOpt := newDefaultLogOptions()
	for _, opt := range options {
		opt.apply(logOpt)
	}

	var optionsZ []zap.Option
	if logOpt.enableCaller {
		// add fileName and LineNum
		caller := zap.AddCaller()
		// skip wrapper
		skip := zap.AddCallerSkip(1)
		optionsZ = append(optionsZ, caller, skip)
	}
	if logOpt.enableDevelopment {
		development := zap.Development()
		optionsZ = append(optionsZ, development)
	}

	return logOpt, optionsZ
}