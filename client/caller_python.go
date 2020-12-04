package client

import (
  "github.com/halokid/ColorfulRabbit"
  "strings"
)

/*
implement caller for python service
 */

type CallerPy struct {
  *Caller
}

func NewCallerPy() CallerIt {
  caller := &Caller{
    typ:    "py",
    selectMode:   RoundRobin,
  }

  return &CallerPy{
    caller,
  }
}

/*
func (c *CallerPy) GetSvcTyp() string {
  return c.typ
}

func (c *CallerPy) selectNode(notGoServers map[string]string) string {
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

func (c *CallerPy) roundRobinSelect(nodesAddr []string) string {
  if len(nodesAddr) == 0 {
    return ""
  }
  nodeIdx := c.nodeIdx
  nodeIdx = nodeIdx % len(nodesAddr)
  c.nodeIdx = nodeIdx + 1

  return genNodeAddr(nodesAddr[nodeIdx])
}
 */

func (c *CallerPy) Invoke(nodeAddr string) ([]byte, error) {
  return []byte{}, nil
}






