package server

import (
  "context"
  "errors"
  logs "github.com/halokid/rpcx-plus/log"
  "github.com/halokid/rpcx-plus/protocol"
  "github.com/halokid/rpcx-plus/share"
  "net/http"
  "net/url"
  "strings"
  "time"
)

func (s *Server) handleHTTP2Request(w http.ResponseWriter, r *http.Request) {
  logs.Debugf("=== Get request from serveByHTTP2, handleHTTP2Request ===")
  ctx := context.WithValue(r.Context(), RemoteConnContextKey, r.RemoteAddr) // notice: It is a string, different with TCP (net.Conn)
  err := s.Plugins.DoPreReadRequest(ctx)
  if err != nil {
    logs.Errorf("DoPreReadRequest err: %+v", err)
    http.Error(w, err.Error(), 500)
    return
  }

  servicePath := r.Header.Get(XServicePath)
  wh := w.Header()
  req, err := HTTPRequest2RpcxRequest(r)
  defer protocol.FreeMsg(req)

  //set headers
  wh.Set(XVersion, r.Header.Get(XVersion))
  wh.Set(XMessageID, r.Header.Get(XMessageID))

  if err == nil && servicePath == "" {
    err = errors.New("empty servicepath")
  } else {
    wh.Set(XServicePath, servicePath)
  }

  if err == nil && r.Header.Get(XServiceMethod) == "" {
    err = errors.New("empty servicemethod")
  } else {
    wh.Set(XServiceMethod, r.Header.Get(XServiceMethod))
  }

  if err == nil && r.Header.Get(XSerializeType) == "" {
    err = errors.New("empty serialized type")
  } else {
    wh.Set(XSerializeType, r.Header.Get(XSerializeType))
  }

  if err != nil {
    rh := r.Header
    for k, v := range rh {
      if strings.HasPrefix(k, "X-RPCX-") && len(v) > 0 {
        wh.Set(k, v[0])
      }
    }

    wh.Set(XMessageStatusType, "Error")
    wh.Set(XErrorMessage, err.Error())
    logs.Errorf("HTTPRequest2RpcxRequest err: %+v", err)
    return
  }
  err = s.Plugins.DoPostReadRequest(ctx, req, nil)
  if err != nil {
    http.Error(w, err.Error(), 500)
    return
  }

  ctx = context.WithValue(ctx, StartRequestContextKey, time.Now().UnixNano())
  err = s.auth(ctx, req)
  if err != nil {
    s.Plugins.DoPreWriteResponse(ctx, req, nil)
    wh.Set(XMessageStatusType, "Error")
    wh.Set(XErrorMessage, err.Error())
    w.WriteHeader(401)
    s.Plugins.DoPostWriteResponse(ctx, req, req.Clone(), err)
    logs.Errorf("s.auth err: %+v", err)
    return
  }

  resMetadata := make(map[string]string)
  newCtx := context.WithValue(context.WithValue(ctx, share.ReqMetaDataKey, req.Metadata),
    share.ResMetaDataKey, resMetadata)

  res, err := s.handleRequest(newCtx, req)
  defer protocol.FreeMsg(res)

  if err != nil {
    logs.Warnf("rpcx: failed to handle gateway request: %v", err)
    wh.Set(XMessageStatusType, "Error")
    wh.Set(XErrorMessage, err.Error())
    w.WriteHeader(500)
    logs.Errorf("s.handleRequest err: %+v", err)
    return
  }

  s.Plugins.DoPreWriteResponse(newCtx, req, nil)
  if len(resMetadata) > 0 { //copy meta in context to request
    meta := res.Metadata
    if meta == nil {
      res.Metadata = resMetadata
    } else {
      for k, v := range resMetadata {
        meta[k] = v
      }
    }
  }

  meta := url.Values{}
  for k, v := range res.Metadata {
    meta.Add(k, v)
  }
  wh.Set(XMeta, meta.Encode())
  logs.Debugf("wh ---- %+v", wh);
  w.Header().Set("Content-Type", "application/json")
  w.Write(res.Payload)
  //w.Write([]byte("service http2 response"))
  s.Plugins.DoPostWriteResponse(newCtx, req, res, err)
}




