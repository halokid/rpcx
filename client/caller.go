package client
/*
service caller
 */

type Caller interface {
  Invoke()  ([]byte, error)
}