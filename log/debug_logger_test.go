package log

import "testing"

func TestAnalyDebug_Print(t *testing.T) {
  ADebug.Print("xxx ---- %+v", "yyy")
}

func TestEnvLogEnable(t *testing.T) {
  logEnable := EnvLogEnable()
  t.Log("logEanble --------", logEnable)
}