package log

import "log"

/**
special code analysis debug print
 */

type AnalyDebug struct {
  Stdout      bool
}

var ADebug *AnalyDebug

func init() {
  ADebug = &AnalyDebug{
    Stdout:   true,
  }
}

func (a *AnalyDebug) Print(format string, v... interface{}) {
  if a.Stdout {
    //log.Println("len v ---------", len(v))
    if len(v) > 0 {
      log.Printf(format, v...)
    } else {
      log.Print(format)
    }
    log.Println()
  }
}