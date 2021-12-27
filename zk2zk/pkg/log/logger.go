package log

import (
	"fmt"
	"go.uber.org/zap"
)

type Logger struct {
	zapLogger *zap.Logger
}

var loggerZ *Logger

func init() {
	zapLog, _ := zap.NewProduction()
	loggerZ = &Logger{zapLogger: zapLog}
}

func NewLogger(options ...Option) *Logger {
	logOpt, optionsZ := buildOpt(options...)
	core := newZapCore(*logOpt)
	return &Logger{zapLogger: zap.New(core, optionsZ...)}
}

func Global() *Logger {
	return loggerZ
}

func (l Logger) HasInitial() bool {
	return l.zapLogger != nil
}

func (l Logger) Real() *zap.Logger {
	return l.zapLogger
}

func (l Logger) Printf(format string, objs ...interface{}) {
	l.zapLogger.Debug(fmt.Sprintf(format, objs...))
}

func (l Logger) Debug(a ...interface{}) {
	l.zapLogger.Debug(printToStr(a))
}

func (l Logger) Info(a ...interface{}) {
	l.zapLogger.Info(printToStr(a))
}

func (l Logger) Warn(a ...interface{}) {
	l.zapLogger.Warn(printToStr(a))
}

func (l Logger) Error(a ...interface{}) {
	l.zapLogger.Error(printToStr(a))
}

func (l Logger) DPanic(a ...interface{}) {
	l.zapLogger.DPanic(printToStr(a))
}

// Log []interface format using fmt.Sprint,
func Debug(a ...interface{}) {
	loggerZ.zapLogger.Debug(printToStr(a))
}

func Info(a ...interface{}) {
	loggerZ.zapLogger.Info(printToStr(a))
}

func Warn(a ...interface{}) {
	loggerZ.zapLogger.Warn(printToStr(a))
}

func Error(a ...interface{}) {
	loggerZ.zapLogger.Error(printToStr(a))
}

func DPanic(a ...interface{}) {
	loggerZ.zapLogger.DPanic(printToStr(a))
}

// zap mode
func DebugZ(msg string, fields ...zap.Field) {
	loggerZ.zapLogger.Debug(msg, fields...)
}

func InfoZ(msg string, fields ...zap.Field) {
	loggerZ.zapLogger.Info(msg, fields...)
}

func WarnZ(msg string, fields ...zap.Field) {
	loggerZ.zapLogger.Warn(msg, fields...)
}

func ErrorZ(msg string, fields ...zap.Field) {
	loggerZ.zapLogger.Error(msg, fields...)
}

func DPanicZ(msg string, fields ...zap.Field) {
	loggerZ.zapLogger.DPanic(msg, fields...)
}

func printToStr(a []interface{}) string {
	return fmt.Sprint(a...)
}
