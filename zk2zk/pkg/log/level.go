package log

import (
	"go.uber.org/zap/zapcore"
	"strings"
)

var levelMapping = map[Level]zapcore.Level{
	DebugLevel:  zapcore.DebugLevel,
	InfoLevel:   zapcore.InfoLevel,
	WarnLevel:   zapcore.WarnLevel,
	ErrorLevel:  zapcore.ErrorLevel,
	DPanicLevel: zapcore.DPanicLevel, // panic when development mode on
}

var stringMapping = map[string]Level{
	"debug":  DebugLevel,
	"info":   InfoLevel,
	"warn":   WarnLevel,
	"error":  ErrorLevel,
	"dPanic": DPanicLevel,
}

type Level string

func MarshalToLevel(l string) (Level, bool) {
	if temp, ok := stringMapping[strings.ToLower(l)]; ok {
		return temp, ok
	} else {
		return DebugLevel, false
	}
}

func (l Level) toZapLevel() (zapcore.Level, bool) {
	if temp, ok := levelMapping[l]; ok {
		return temp, ok
	} else {
		return zapcore.DebugLevel, false
	}
}

func (l Level) String() string {
	return string(l)
}
