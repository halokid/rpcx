package log

import (
	"fmt"
	"log"
	"os"

	"github.com/fatih/color"
)

type defaultLogger struct {
	*log.Logger
}

func (l *defaultLogger) Error(v ...interface{}) {
	if CheckLogLevel(ErrorLevel) {
		l.Output(calldepth, header(color.RedString("ERROR"), fmt.Sprint(v...)))
	}
}

func (l *defaultLogger) Errorf(format string, v ...interface{}) {
	if CheckLogLevel(ErrorLevel) {
		l.Output(calldepth, header(color.RedString("ERROR"), fmt.Sprintf(format, v...)))
	}
}

func (l *defaultLogger) ErrorCheck(err error, v ...interface{}) {
	if CheckLogLevel(ErrorLevel) && err != nil {
		l.Output(calldepth, header(color.RedString("ERROR"), fmt.Sprint(v...)))
	}
}

func (l *defaultLogger) ErrorfCheck(err error, format string, v ...interface{}) {
	if CheckLogLevel(ErrorLevel) && err != nil {
		l.Output(calldepth, header(color.RedString("ERROR"), fmt.Sprintf(format, v...)))
	}
}

func (l *defaultLogger) Warn(v ...interface{}) {
	if CheckLogLevel(WarnLevel) {
		l.Output(calldepth, header(color.YellowString("WARN "), fmt.Sprint(v...)))
	}
}

func (l *defaultLogger) Warnf(format string, v ...interface{}) {
	if CheckLogLevel(WarnLevel) {
		l.Output(calldepth, header(color.YellowString("WARN "), fmt.Sprintf(format, v...)))
	}
}

func (l *defaultLogger) Info(v ...interface{}) {
	if CheckLogLevel(InfoLevel) {
		l.Output(calldepth, header(color.GreenString("INFO "), fmt.Sprint(v...)))
	}
}

func (l *defaultLogger) Infof(format string, v ...interface{}) {
	if CheckLogLevel(InfoLevel) {
		l.Output(calldepth, header(color.GreenString("INFO "), fmt.Sprintf(format, v...)))
	}
}

func (l *defaultLogger) Debug(v ...interface{}) {
	//l.Output(calldepth, header("DEBUG", fmt.Sprint(v...)))
	if CheckLogLevel(DebugLevel) {
		l.Output(calldepth, header(color.CyanString("DEBUG "), fmt.Sprint(v...)))
	}
}

func (l *defaultLogger) Debugf(format string, v ...interface{}) {
	//l.Output(calldepth, header("DEBUG", fmt.Sprintf(format, v...)))
	if CheckLogLevel(DebugLevel) {
		l.Output(calldepth, header(color.CyanString("DEBUG "), fmt.Sprintf(format, v...)))
	}
}

func (l *defaultLogger) Trace(v ...interface{}) {
	//l.Output(calldepth, header("DEBUG", fmt.Sprint(v...)))
	if CheckLogLevel(TraceLevel) {
		l.Output(calldepth, header(color.BlueString("DEBUG "), fmt.Sprint(v...)))
	}
}

func (l *defaultLogger) Tracef(format string, v ...interface{}) {
	//l.Output(calldepth, header("DEBUG", fmt.Sprintf(format, v...)))
	if CheckLogLevel(TraceLevel) {
		l.Output(calldepth, header(color.BlueString("DEBUG "), fmt.Sprintf(format, v...)))
	}
}

func (l *defaultLogger) Fatal(v ...interface{}) {
	l.Output(calldepth, header(color.MagentaString("FATAL"), fmt.Sprint(v...)))
	os.Exit(1)
}

func (l *defaultLogger) Fatalf(format string, v ...interface{}) {
	l.Output(calldepth, header(color.MagentaString("FATAL"), fmt.Sprintf(format, v...)))
	os.Exit(1)
}

func (l *defaultLogger) Panic(v ...interface{}) {
	l.Logger.Panic(v...)
}

func (l *defaultLogger) Panicf(format string, v ...interface{}) {
	l.Logger.Panicf(format, v...)
}

func (l *defaultLogger) Handle(v ...interface{}) {
	l.Error(v...)
}

func header(lvl, msg string) string {
	return fmt.Sprintf("%s: %s", lvl, msg)
}
