package main

import (
  "fmt"
  "time"
)

func main() {
  dying := make(chan struct{})
  tc := time.NewTicker(time.Duration(3 * time.Second))

  //for {
  //  fmt.Println("testing ...")
  //}

  go func() {
    time.Sleep(10 * time.Second)
    dying <- struct{}{}
  }()

  for {
    select {
    case <-dying:
      fmt.Println("select get dying")
      return
    case <-tc.C:
      fmt.Println("waiting dying....")
    }
  }
}
