package client
/*
implement caller for python service
*/

import (
  "encoding/json"
  "github.com/mozillazg/request"
  "log"
  "net/http"
  "time"
)

func (c *caller) invokePy(nodeAddr string, svc string, call string, bodyTran map[string]interface{}) ([]byte, error) {
  url := "http://" + nodeAddr + "/api"
  log.Println("url -----------", url)
  hc := &http.Client{}
  timeOut := time.Duration(3 * time.Second)
  hc.Timeout = timeOut
  req := request.NewRequest(hc)
  //req.Headers["POMX-Svc"] = svc
  //req.Headers["POMX-Call"] = call
  log.Println("bodyTran ---------------", bodyTran)
  req.Json = map[string]interface{} {
    "method": svc + "." + call,
    //"params": map[string]string{"name":"jimmy"},
    "params": bodyTran,
    "id": "1",
  }
  log.Println("req.Json -------------------- ", req.Json)
  rsp, err := req.Post(url)
  if err != nil {
    log.Println("请求python服务失败")
  }
  //rspData, err := ioutil.ReadAll(rsp.Body)
  j, err := rsp.Json()
  log.Println("rsp json --------", j)
  defer rsp.Body.Close()
  jb := j.Get("result").Interface()
  //return []byte(rspData)
  log.Println("jb ----------", jb)
  //return []byte(jb)
  //return []byte("")
  b, err := json.Marshal(jb)
  return b, nil
}



