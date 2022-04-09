package client
/*
implement caller for python service
*/

import (
  "encoding/json"
  "github.com/mozillazg/request"
  "net/http"
  "time"
  
  logs "github.com/halokid/rpcx-plus/log"
)

func (c *caller) invokePy(nodeAddr string, svc string, call string, bodyTran map[string]interface{}) ([]byte, error) {
  url := "http://" + nodeAddr + "/api"
  logs.Debug("url -----------", url)
  hc := &http.Client{}
  timeOut := time.Duration(15 * time.Second)
  hc.Timeout = timeOut
  req := request.NewRequest(hc)
  //req.Headers["POMX-Svc"] = svc
  //req.Headers["POMX-Call"] = call
  logs.Debug("bodyTran ---------------", bodyTran)
  req.Json = map[string]interface{} {
    "method": svc + "." + call,
    //"params": map[string]string{"name":"jimmy"},
    "params": bodyTran,
    "id": "1",
  }
  logs.Debug("req.Json -------------------- ", req.Json)
  rsp, err := req.Post(url)
  if err != nil {
    logs.Debug("请求python服务失败")
  }
  //rspData, err := ioutil.ReadAll(rsp.Body)
  j, err := rsp.Json()
  logs.Debug("rsp json --------", j)
  defer rsp.Body.Close()
  jb := j.Get("result").Interface()
  //return []byte(rspData)
  logs.Debug("jb ----------", jb)
  //return []byte(jb)
  //return []byte("")
  b, err := json.Marshal(jb)
  return b, nil
}



