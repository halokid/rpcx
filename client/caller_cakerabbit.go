package client
/*
implement caller for cakeRabbit service, use msgpack
*/

import (
  "github.com/halokid/ColorfulRabbit"
  "github.com/halokid/msgpack-rpc-go/rpc"
  logs "github.com/halokid/rpcx-plus/log"
  "github.com/spf13/cast"
  logx "log"
  "net"
)

func (c *caller) invokeCake(nodeAddr string, svc string, call string, bodyTran map[string]interface{}, psKey []string) ([]byte, error) {
  conn, err := net.Dial("tcp", nodeAddr)
  defer conn.Close()
  ColorfulRabbit.CheckError(err, "invoke cakeRabbit service error")
  if err != nil {
    return []byte(`{"msg": "cannot find svc nodes || svc return exception"} code 1`), err
  }

  client := rpc.NewSession(conn, true)
  var args []interface{}
  //paramsIndex := []string{"pageIndex", "pageSize", "keyword"}
  paramsIndex := psKey

  for _, key := range paramsIndex {
    if val, ok := bodyTran[key]; ok {
      args = append(args, cast.ToString(val))
    }
  }
  // todo: 如果是非自定义路由， 则重新读取body的json数据， 不依赖于pkKey
  if len(args) == 0 {
    for _, arg := range bodyTran {
      args = append(args, cast.ToString(arg))
    }
  }

  logx.Printf("invokeCake args ----------- %+v", args)
  //rsp, err := client.Send("say_hello", "foo")
  //rsp, err := client.Send(call, "foo")
  //rsp, err := client.Send(call, 3)
  // todo: change type reflect in cakeRabbit, not in golang, so bodyTran all params use string type
  rsp, err := client.Send(call, args...)
  ColorfulRabbit.CheckError(err, "invoke cakeRabbit service call error")
  if err != nil {
    return []byte(`{"msg": "cannot find svc nodes || svc return exception"} code 2}`), err
  }
  logs.Info("invokeCake rsp -------", rsp)
  rspS := rsp.String()
  return []byte(rspS), nil
}



