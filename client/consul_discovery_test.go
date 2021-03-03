package client

import (
  "testing"
  "time"
)

func TestWatchDiscovery(t *testing.T) {
  i := 0
readChanges:
  for {
    i += 1
    t.Log("ix ------------", i)
    if i > 5 {
      t.Log("iy ------------", i)
      return // todo: 退出TestWatchDiscovery, 退出整个函数
    }

    if i > 3 {
      break readChanges
    }
  }

  t.Log("i final ------------", i)
}

func TestComm(t *testing.T) {
  i := 0
  //ch := make(chan int)
  for {
    i += 1
    //ch <-i
    t.Log("ix ------------", i)

  readChanges:
    for {
      if i > 0 {
        time.Sleep(3 * time.Second)
        t.Logf("quit")
        break readChanges
      }

      /*
      select {
      case i := <-ch:
        t.Logf("i in readChanges -------------- %+v", i)
        break readChanges
      default:
        time.Sleep(3 * time.Second)
      }
      */
    }
    t.Log("iy ------------", i)

    t.Log("执行的其实是for循环 ------------", i)
  }
}
