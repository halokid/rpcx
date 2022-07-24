package main

import "fmt"

const (
  i = 7
  j
  k
)

type S struct {
  name  string
}

func main() {
  //fmt.Println(i, j, k)

  //m := map[string]S{"x": S{"one"}}
  //m["x"].name = "two"
  //fmt.Println(m["x"])

  //a := []string{"a", "b"}
  //a = a[:0]
  //fmt.Println(a, len(a), cap(a))

  var s []int
  a := [...]int{1, 2, 3, 4, 5, 6, 7 , 8, 9}
  s = a[2:4]
  //newS := append(s, 55, 66)
  //fmt.Println(len(newS), cap(newS))
  fmt.Println(cap(s))
  //s[0] = 22
  //fmt.Println(a)

  //var s *int
  //fmt.Println(s)

  //s2 := []int{1, 2, 3}
  //s3 := []int{4, 5, 6, 7}
  //n2 := copy(s2, s3)
  //fmt.Println(n2, s2, s3)
}

