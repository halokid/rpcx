package client

import (
  "github.com/halokid/ColorfulRabbit"
  "log"
  "strings"
)

/*
service caller
 */

type Caller interface {
  GetSvcTyp()   string
  Call(notGoServers map[string]string, svc string, call string, bodyTran map[string]interface{}) ([]byte, error)
}

type caller struct {
  typ               string
  selectMode        SelectMode
  failMode          FailMode
  nodeIdx           int
  option            callerOption
}

type callerOption struct {
  Retries     int
}

func NewCaller(svcTyp string) Caller {
  return &caller{
    typ:   svcTyp,
    selectMode: RoundRobin,
    option:     callerOption{Retries: 3},
  }
}

func (c *caller) GetSvcTyp() string {
  return c.typ
}

func (c *caller) Call(notGoServers map[string]string, svc string, call string, bodyTran map[string]interface{}) ([]byte, error) {

  nodeAddr := c.selectNode(notGoServers)
  log.Printf("nodeAddr --------- %+v", nodeAddr)
  b, err := c.invoke(nodeAddr, svc, call, bodyTran)
  return b, err
}

func (c *caller) selectNode(notGoServers map[string]string) string {
  nodeAddr := ""
  // get nodes addr
  nodesAddr := ColorfulRabbit.GetKeysSs(notGoServers)
  switch c.selectMode {
  case RoundRobin:
    nodeAddr = c.roundRobinSelect(nodesAddr)
  }
  return nodeAddr
}

func (c *caller) invoke(nodeAddr string, svc string, call string, bodyTran map[string]interface{}) ([]byte, error) {

  switch c.failMode {

  case Failover:
    retries := c.option.Retries
    var b []byte
    var err error
    for retries >= 0 {
      log.Println("Failtry once---")
      retries--
      b, err = c.invokeWrap(nodeAddr, svc, call, bodyTran)
      if err == nil {
        return b, err
      }
    }
  }

  return []byte{}, nil
}

func (c *caller) invokeWrap(nodeAddr string, svc string, call string, bodyTran map[string]interface{}) ([]byte, error) {
  var b []byte
  var err error

  switch c.typ {
  case "py":
    b, err = c.invokePy(nodeAddr, svc, call, bodyTran)
  case "rust":
    b, err = c.invokeRust(nodeAddr, svc, call, bodyTran)
  case "cakeRabbit":
    b, err = c.invokeCake(nodeAddr, svc, call, bodyTran)

  }

  return b, err
}

func genNodeAddr(nodeAddrDirty string) string {
  // change tcp@xxx to xxx
  nodeAddrSp := strings.Split(nodeAddrDirty, "@")
  return nodeAddrSp[1]
}

func (c *caller) roundRobinSelect(nodesAddr []string) string {
  if len(nodesAddr) == 0 {
    return ""
  }
  nodeIdx := c.nodeIdx
  nodeIdx = nodeIdx % len(nodesAddr)
  c.nodeIdx = nodeIdx + 1

  return genNodeAddr(nodesAddr[nodeIdx])
}



