package log

// todo: discard!!!

/**
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
  logs.Println("Env logEnable -->>>", logEnable)
  if logEnable == "true" {
    return true
  }
  return false
}

func (a *AnalyDebug) Print(format string, v ...interface{}) {
  if a.Stdout {
    //if EnvLogEnable() {
    //logs.Println("len v ---------", len(v))
    mutex := sync.RWMutex{}
    mutex.Lock()
    if len(v) > 0 {
      logs.Printf(format, v...)
    } else {
      logs.Print(format)
    }
    mutex.Unlock()
    logs.Println()
  }
}

func (a *AnalyDebug) PrintErr(format string, v ...interface{}) {
  mutex := sync.RWMutex{}
  mutex.Lock()
  if len(v) > 0 {
    logs.Printf(format, v...)
  } else {
    logs.Print(format)
  }
  mutex.Unlock()
  logs.Println()
}
*/




