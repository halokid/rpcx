package client

import "testing"

func TestWatchDiscovery(t *testing.T) {
  i := 0
  readChanges:
    for {
      i += 1
      t.Log("ix ------------", i)
      if i > 5 {
        t.Log("iy ------------", i)
        return      // todo: 退出TestWatchDiscovery, 退出整个函数
      }

      if i > 3 {
        break readChanges
      }
    }

  t.Log("i final ------------", i)
}


