package client

import (
  "github.com/halokid/ColorfulRabbit"
  "strings"
)

/*
service caller
 */

type CallerIt interface {
  GetSvcTyp()   string
  selectNode(notGoServers map[string]string)  string
  Invoke(nodeAddr string)  ([]byte, error)
}

func SelectCaller(xc *xClient) CallerIt {
  switch xc.svcTyp {
  case "py":
    return NewCallerPy()
  case "rust":
  }

  return nil
}

type Caller struct {
  typ               string
  selectMode        SelectMode
  nodeIdx           int
}

func (c *Caller) GetSvcTyp() string {
  return c.typ
}

func (c *Caller) selectNode(notGoServers map[string]string) string {
  node := ""
  // get nodes addr
  nodesAddr := ColorfulRabbit.GetKeysSs(notGoServers)
  switch c.selectMode {
  case RoundRobin:
    node = c.roundRobinSelect(nodesAddr)
  }
  return node
}

func genNodeAddr(nodeAddrDirty string) string {
  // change tcp@xxx to xxx
  nodeAddrSp := strings.Split(nodeAddrDirty, "@")
  return nodeAddrSp[1]
}

func (c *Caller) roundRobinSelect(nodesAddr []string) string {
  if len(nodesAddr) == 0 {
    return ""
  }
  nodeIdx := c.nodeIdx
  nodeIdx = nodeIdx % len(nodesAddr)
  c.nodeIdx = nodeIdx + 1

  return genNodeAddr(nodesAddr[nodeIdx])
}



