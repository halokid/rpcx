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

func (a *AnalyDebug) Print(i... interface{}) {
  if a.Stdout {
    log.Printf("%+v", i)
  }
}