package client
/*
implement caller for cakeRabbit service
*/

import (
  "github.com/halokid/ColorfulRabbit"
  "github.com/halokid/rpcx-plus/log"
  "github.com/msgpack-rpc/msgpack-rpc-go/rpc"
  "net"
)

func (c *caller) invokeCake(nodeAddr string, svc string, call string, bodyTran map[string]interface{}) ([]byte, error) {
  conn, err := net.Dial("tcp", nodeAddr)
  defer conn.Close()
  ColorfulRabbit.CheckError(err, "invoke cakeRabbit service error")
  if err != nil {
    return []byte{}, err
  }

  client := rpc.NewSession(conn, true)
  var args []interface{}
  for _, arg := range bodyTran {
    args = append(args, arg)
  }
  //rsp, err := client.Send("say_hello", "foo")
  //rsp, err := client.Send(call, "foo")
  rsp, err := client.Send(call, args...)
  ColorfulRabbit.CheckError(err, "invoke cakeRabbit service call error")
  if err != nil {
    return []byte{}, err
  }
  log.ADebug.Print("invokeCake rsp -------", rsp)
  rspS := rsp.String()
  return []byte(rspS), nil
}



