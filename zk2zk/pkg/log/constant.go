package log

const (
	// log output path
	defaultLogFilePath = "./out.log"
	// maximum MB saved per log file
	defaultLogMaxSize = 100
	// maximum number of log files saved
	defaultLogMaxBackups = 30
	// maximum number of days the log saved
	defaultLogMaxAge = 0
)

const (
	DebugLevel  Level = "debug"
	InfoLevel   Level = "info"
	WarnLevel   Level = "warn"
	ErrorLevel  Level = "error"
	DPanicLevel Level = "dPanic"
)
