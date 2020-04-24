package main

import (
  "testing"
)

func TestChDying(t *testing.T) {
  //dying := make(chan struct{})

  //tc := time.NewTicker(time.Duration(3 * time.Second))
  /**
  for {
    select {
    case <-dying:
      t.Log("select get dying")
    case <-tc.C:
      t.Log("waiting dying....")
    }
  }
  */

  for {
    t.Log("test .....")
  }
}