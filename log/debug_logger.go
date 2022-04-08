package log

// todo: discard!!!

///**
import (
  "log"
  "os"
  "sync"
)

// special code analysis debug print

type AnalyDebug struct {
  Stdout bool
}

var ADebug *AnalyDebug

func init() {
  stdout := EnvLogEnable()
  ADebug = &AnalyDebug{
    //Stdout:   true,
    Stdout: stdout,
  }
}

func EnvLogEnable() bool {
  logEnable := os.Getenv("RPCX_PLUS_LOG")
  log.Println("logEnable ------------", logEnable)
  if logEnable == "true" {
    return true
  }
  return false
}

func (a *AnalyDebug) Print(format string, v ...interface{}) {
  if a.Stdout {
    //if EnvLogEnable() {
    //log.Println("len v ---------", len(v))
    mutex := sync.RWMutex{}
    mutex.Lock()
    if len(v) > 0 {
      log.Printf(format, v...)
    } else {
      log.Print(format)
    }
    mutex.Unlock()
    log.Println()
  }
}

func (a *AnalyDebug) PrintErr(format string, v ...interface{}) {
  mutex := sync.RWMutex{}
  mutex.Lock()
  if len(v) > 0 {
    log.Printf(format, v...)
  } else {
    log.Print(format)
  }
  mutex.Unlock()
  log.Println()
}
//*/




