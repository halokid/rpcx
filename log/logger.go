package log

import (
	"log"
	"os"
)

const (
	calldepth = 3
)

type LogLevel int

const (
	InfoLevel LogLevel = iota
	DebugLevel
	WarnLevel
	ErrorLevel
)

// todo: define the logger adapter
var l Logger = &defaultLogger{log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)}
var LogLevelEnv LogLevel 		// default is info

func CheckLogLevel(logLevel LogLevel) bool {
	//if LogLevelEnv <= logLevel {
	if logLevel >= LogLevelEnv {
		return true
	}
	return false
}

// initialize the logLevel, default is info
func init() {
	logLevel := os.Getenv("RPCX_PLUS_LOG")
	switch logLevel {
	case "info":
		LogLevelEnv = InfoLevel
	case "debug":
		LogLevelEnv = DebugLevel
	case "warn":
		LogLevelEnv = WarnLevel
	case "error":
		LogLevelEnv = ErrorLevel
	default:
		LogLevelEnv = InfoLevel
	}
	log.Println("LogLevelEnv -->>>", LogLevelEnv)
}

type Logger interface {
	Debug(v ...interface{})
	Debugf(format string, v ...interface{})

	Info(v ...interface{})
	Infof(format string, v ...interface{})

	Warn(v ...interface{})
	Warnf(format string, v ...interface{})

	Error(v ...interface{})
	Errorf(format string, v ...interface{})

	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})

	Panic(v ...interface{})
	Panicf(format string, v ...interface{})
}

type Handler interface {
	Handle(v ...interface{})
}

func SetLogger(logger Logger) {
	l = logger
}

func SetDummyLogger() {
	l = &dummyLogger{}
}

func Debug(v ...interface{}) {
	l.Debug(v...)
}
func Debugf(format string, v ...interface{}) {
	l.Debugf(format, v...)
}

func Info(v ...interface{}) {
	l.Info(v...)
}
func Infof(format string, v ...interface{}) {
	l.Infof(format, v...)
}

func Warn(v ...interface{}) {
	l.Warn(v...)
}
func Warnf(format string, v ...interface{}) {
	l.Warnf(format, v...)
}

func Error(v ...interface{}) {
	l.Error(v...)
}
func Errorf(format string, v ...interface{}) {
	l.Errorf(format, v...)
}

func Fatal(v ...interface{}) {
	l.Fatal(v...)
}
func Fatalf(format string, v ...interface{}) {
	l.Fatalf(format, v...)
}

func Panic(v ...interface{}) {
	l.Panic(v...)
}
func Panicf(format string, v ...interface{}) {
	l.Panicf(format, v...)
}

func Handle(v ...interface{}) {
	if handle, ok := l.(Handler); ok {
		handle.Handle(v...)
	}
}
