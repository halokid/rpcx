package client
/*
implement caller for rust GRPC service
*/

import (
  "encoding/json"
  "reflect"

  pbx "github.com/halokid/rpcx-plus/client/proto"
  "golang.org/x/net/context"
  "google.golang.org/grpc"

  logs "github.com/halokid/rpcx-plus/log"
)

// call rust grpc service
func (c *caller) invokeRust(nodeAddr string, svc string, call string, bodyTran map[string]interface{}) ([]byte, error) {
  conn, err := grpc.Dial(nodeAddr, grpc.WithInsecure())
  if err != nil {
    logs.Fatalf("did not connect: %v", err)
  }
  defer conn.Close()
  cx := pbx.NewPorsClient(conn)

  // call svc.method
  dataJs, _ := json.Marshal(bodyTran)
  reqData := `{"call": "` + svc+"."+call + `", "data": ` + string(dataJs) + `}`
  rsp, err := cx.Invoke(context.Background(), &pbx.Req{Reqdata: reqData})

  if err != nil {
    logs.Fatalf("could not greet: %v", err)
  }
  logs.Debugf("say_hi---rsp type: %+v, struct: %+v, val: %+v", reflect.TypeOf(rsp), rsp, rsp.Rspdata)

  return []byte(rsp.Rspdata), nil
}






