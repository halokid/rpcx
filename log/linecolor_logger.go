package log

import (
	"fmt"
	"log"
	"os"
)

type LineColorLogger struct {
	*log.Logger
}

func (l *LineColorLogger) Error(v ...interface{}) {
	if CheckLogLevel(ErrorLevel) {
		l.Output(calldepth, lineColorHeader("\033[0;31m", "ERROR", fmt.Sprint(v...)))
	}
}

func (l *LineColorLogger) Errorf(format string, v ...interface{}) {
	if CheckLogLevel(ErrorLevel) {
		l.Output(calldepth, lineColorHeader("\033[0;31m", "ERROR", fmt.Sprintf(format, v...)))
	}
}

func (l *LineColorLogger) ErrorCheck(err error, v ...interface{}) {
	if CheckLogLevel(ErrorLevel) && err != nil {
		l.Output(calldepth, lineColorHeader("\033[0;31m", "ERROR", fmt.Sprint(v...)))
	}
}

func (l *LineColorLogger) ErrorfCheck(err error, format string, v ...interface{}) {
	if CheckLogLevel(ErrorLevel) && err != nil {
		l.Output(calldepth, lineColorHeader("\033[0;31m", "ERROR", fmt.Sprintf(format, v...)))
	}
}

func (l *LineColorLogger) Warn(v ...interface{}) {
	if CheckLogLevel(WarnLevel) {
		l.Output(calldepth, lineColorHeader("\033[0;35m", "WARN ", fmt.Sprint(v...)))
	}
}

func (l *LineColorLogger) Warnf(format string, v ...interface{}) {
	if CheckLogLevel(WarnLevel) {
		l.Output(calldepth, lineColorHeader("\033[0;35m", "WARN ", fmt.Sprintf(format, v...)))
	}
}

func (l *LineColorLogger) Info(v ...interface{}) {
	if CheckLogLevel(InfoLevel) {
		//l.Output(calldepth, lineColorHeader("\033[0;32m", "INFO ", fmt.Sprint(v...)))
		l.Output(calldepth, lineColorHeader("\033[1;32m", "INFO ", fmt.Sprint(v...)))
	}
}

func (l *LineColorLogger) Infof(format string, v ...interface{}) {
	if CheckLogLevel(InfoLevel) {
		//l.Output(calldepth, lineColorHeader("\033[0;32m", "INFO ", fmt.Sprintf(format, v...)))
		l.Output(calldepth, lineColorHeader("\033[1;32m", "INFO ", fmt.Sprintf(format, v...)))
	}
}

func (l *LineColorLogger) Debug(v ...interface{}) {
	//l.Output(calldepth, header("DEBUG", fmt.Sprint(v...)))
	if CheckLogLevel(DebugLevel) {
		l.Output(calldepth, lineColorHeader("\033[0;33m", "DEBUG ", fmt.Sprint(v...)))
	}
}

func (l *LineColorLogger) Debugf(format string, v ...interface{}) {
	//l.Output(calldepth, header("DEBUG", fmt.Sprintf(format, v...)))
	if CheckLogLevel(DebugLevel) {
		l.Output(calldepth, lineColorHeader("\033[0;33m", "DEBUG ", fmt.Sprintf(format, v...)))
	}
}

func (l *LineColorLogger) Trace(v ...interface{}) {
	//l.Output(calldepth, header("DEBUG", fmt.Sprint(v...)))
	if CheckLogLevel(TraceLevel) {
		l.Output(calldepth, lineColorHeader("\033[0;31m", "TRACE ", fmt.Sprint(v...)))
	}
}

func (l *LineColorLogger) Tracef(format string, v ...interface{}) {
	//l.Output(calldepth, header("DEBUG", fmt.Sprintf(format, v...)))
	if CheckLogLevel(TraceLevel) {
		l.Output(calldepth, lineColorHeader("\033[0;31m", "TRACE ", fmt.Sprintf(format, v...)))
	}
}

func (l *LineColorLogger) Fatal(v ...interface{}) {
	l.Output(calldepth, lineColorHeader("\033[0;35m", "FATAL", fmt.Sprint(v...)))
	os.Exit(1)
}

func (l *LineColorLogger) Fatalf(format string, v ...interface{}) {
	l.Output(calldepth, lineColorHeader("\033[0;35m", "FATAL", fmt.Sprintf(format, v...)))
	os.Exit(1)
}

func (l *LineColorLogger) Panic(v ...interface{}) {
	l.Logger.Panic(v...)
}

func (l *LineColorLogger) Panicf(format string, v ...interface{}) {
	l.Logger.Panicf(format, v...)
}

func (l *LineColorLogger) Handle(v ...interface{}) {
	l.Error(v...)
}

func lineColorHeader(colorStart, lvl, msg string) string {
	return fmt.Sprintf("%s [%s]: %s \033[0m", colorStart, lvl, msg)
}




