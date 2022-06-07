package client

import (
  "bufio"
  "bytes"
  "context"
  "crypto/tls"
  "encoding/json"
  "errors"
  "fmt"
  logs "github.com/halokid/rpcx-plus/log"
  "github.com/halokid/rpcx-plus/protocol"
  "github.com/halokid/rpcx-plus/share"
  "github.com/opentracing/opentracing-go"
  "github.com/rubyist/circuitbreaker"
  "go.opencensus.io/trace"
  "golang.org/x/net/http2"
  "io"
  "io/ioutil"
  //"log"
  "net"
  "net/http"
  "net/url"
  "reflect"
  "strconv"
  "strings"
  "sync"
  "time"
  "unsafe"
)

const (
  XVersion           = "X-RPCX-Version"
  XMessageType       = "X-RPCX-MesssageType"
  XHeartbeat         = "X-RPCX-Heartbeat"
  XOneway            = "X-RPCX-Oneway"
  XMessageStatusType = "X-RPCX-MessageStatusType"
  XSerializeType     = "X-RPCX-SerializeType"
  XMessageID         = "X-RPCX-MessageID"
  XServicePath       = "X-RPCX-ServicePath"
  XServiceMethod     = "X-RPCX-ServiceMethod"
  XMeta              = "X-RPCX-Meta"
  XErrorMessage      = "X-RPCX-ErrorMessage"
)

// ServiceError is an error from server.
type ServiceError string

func (e ServiceError) Error() string {
  return string(e)
}

// DefaultOption is a common option configuration for client.
var DefaultOption = Option{
  Retries:        3,
  RPCPath:        share.DefaultRPCPath,
  ConnectTimeout: 10 * time.Second,
  //ConnectTimeout: 2 * time.Second,
  SerializeType:  protocol.MsgPack,
  //SerializeType:  protocol.JSON,
  CompressType:   protocol.None,
  BackupLatency:  10 * time.Millisecond,
}

// Breaker is a CircuitBreaker interface.
type Breaker interface {
  Call(func() error, time.Duration) error
  Fail()
  Success()
  Ready() bool
}

// CircuitBreaker is a default circuit breaker (RateBreaker(0.95, 100)).
var CircuitBreaker Breaker = circuit.NewRateBreaker(0.95, 100)

// ErrShutdown connection is closed.
var (
  ErrShutdown         = errors.New("connection is shut down")
  ErrUnsupportedCodec = errors.New("unsupported codec")
)

const (
  // ReaderBuffsize is used for bufio reader.
  ReaderBuffsize = 16 * 1024
  // WriterBuffsize is used for bufio writer.
  WriterBuffsize = 16 * 1024
)

type seqKey struct{}

// RPCClient is interface that defines one client to call one server.
type RPCClient interface {
  Connect(network, address string) error
  Go(ctx context.Context, servicePath, serviceMethod string, args interface{}, reply interface{}, done chan *Call) *Call
  Call(ctx context.Context, servicePath, serviceMethod string, args interface{}, reply interface{}) error
  SendRaw(ctx context.Context, r *protocol.Message) (map[string]string, []byte, error)
  Close() error

  RegisterServerMessageChan(ch chan<- *protocol.Message)
  UnregisterServerMessageChan()

  IsClosing() bool
  IsShutdown() bool

  SetHttp2SvcNode(k string) error
  Http2CallSendRaw(ctx context.Context, r *protocol.Message) (map[string]string, []byte, error)
  Http2Call(ctx context.Context, servicePath, serviceMethod string, args interface{}, reply interface{}) error
  Http2CallGw(ctx context.Context, servicePath, serviceMethod string, args interface{}, reply interface{}) error
}

// Client represents a RPC client.
type Client struct {
  option Option

  Conn net.Conn
  r    *bufio.Reader
  //w    *bufio.Writer

  mutex        sync.Mutex // protects following
  seq          uint64
  pending      map[uint64]*Call
  closing      bool // user has called Close
  shutdown     bool // server has told us to stop
  pluginClosed bool // the plugin has been called

  Plugins PluginContainer

  ServerMessageChan chan<- *protocol.Message

  Http2SvcNode string
}

// NewClient returns a new Client with the option.
func NewClient(option Option) *Client {
  return &Client{
    option: option,
  }
}

// Option contains all options for creating clients.
type Option struct {
  // Group is used to select the services in the same group. Services set group info in their meta.
  // If it is empty, clients will ignore group.
  Group string

  // Retries retries to send
  Retries int

  // TLSConfig for tcp and quic
  TLSConfig *tls.Config
  // kcp.BlockCrypt
  Block interface{}
  // RPCPath for http connection
  RPCPath string
  //ConnectTimeout sets timeout for dialing
  ConnectTimeout time.Duration
  // ReadTimeout sets readdeadline for underlying net.Conns
  ReadTimeout time.Duration
  // WriteTimeout sets writedeadline for underlying net.Conns
  WriteTimeout time.Duration

  // BackupLatency is used for Failbackup mode. rpcx will sends another request if the first response doesn't return in BackupLatency time.
  BackupLatency time.Duration

  // Breaker is used to config CircuitBreaker
  GenBreaker func() Breaker

  SerializeType protocol.SerializeType
  CompressType  protocol.CompressType

  Heartbeat         bool
  HeartbeatInterval time.Duration

  Http2             bool
}

// Call represents an active RPC.
type Call struct {
  ServicePath   string            // The name of the service and method to call.
  ServiceMethod string            // The name of the service and method to call.
  Metadata      map[string]string //metadata
  ResMetadata   map[string]string
  Args          interface{} // The argument to the function (*struct).
  Reply         interface{} // The reply from the function (*struct).
  Error         error       // After completion, the error status.
  Done          chan *Call  // Strobes when call is complete.
  Raw           bool        // raw message or not
}

func (call *Call) done() {
  select {
  case call.Done <- call:       // todo: 是这里重写了SendRaw的 call.Done
    // ok
  default:
    //logs.Debug("rpc: discarding Call reply due to insufficient Done chan capacity")
    logs.Debug("rpc: 因 call.Done 无法接收到信号，该请求无法完成，无法获取reply")
  }
}

// RegisterServerMessageChan registers the channel that receives server requests.
func (client *Client) RegisterServerMessageChan(ch chan<- *protocol.Message) {
  client.ServerMessageChan = ch
}

// UnregisterServerMessageChan removes ServerMessageChan.
func (client *Client) UnregisterServerMessageChan() {
  client.ServerMessageChan = nil
}

// IsClosing client is closing or not.
func (client *Client) IsClosing() bool {
  client.mutex.Lock()
  defer client.mutex.Unlock()
  return client.closing
}

// IsShutdown client is shutdown or not.
func (client *Client) IsShutdown() bool {
  client.mutex.Lock()
  defer client.mutex.Unlock()
  return client.shutdown
}

func (client *Client) SetHttp2SvcNode(k string) error {
  logs.Debugf("-->>> client SetHttp2SvcNode...")
  svcNode := strings.Split(k, "@")
  if len(svcNode) > 1 {
    client.Http2SvcNode = svcNode[1]
  } else {
    client.Http2SvcNode = k
  }
  logs.Debugf("after SetHttp2SvcNode client -->>> %+v", client)
  return nil
}

func (client *Client) Http2Call(ctx context.Context, servicePath, serviceMethod string, args interface{},
  reply interface{}) error {
  logs.Debugf("=== call Http2Service Http2Call ===")
  /*
  // todo: for test ---------------------------
  data := `{"Greet": "hello-world"}`
  err := json.Unmarshal([]byte(data), reply)
  logs.Debugf("err: %+v, reply -->>> %+v", err, reply)
  return nil
  // -----------------------------------------
  */

  //t1 := &http.Transport{
  //  ExpectContinueTimeout: 3*time.Second, // for example
  //}
  //http2.ConfigureTransport(t1)

  clientx := http.Client{
    Transport:     &http2.Transport{
      AllowHTTP:  true,
      DialTLS: func(network, addr string, cfg *tls.Config) (conn net.Conn, err error) {
        return net.Dial(network, addr)
      },
    },
    Timeout: 3 * time.Second,
  }

  // todo: http2 client request timeout is not the same as http1
  //timeout := time.Duration(Http2CallTimeout) * time.Second
  //clientx.Timeout = timeout
  //url := "http://127.0.0.1:8972"
  url := fmt.Sprintf("http://%s", client.Http2SvcNode)
  //payload, err := json.Marshal(map[string]string {
  //  "name": "halokid",
  //})
  payload, err := json.Marshal(args)
  //payload := args.([]byte)
  //var err error
  logs.ErrorfCheck(err, "http2 client serialize payload error, args -->>> %+v", args)

  req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
  req.Header.Add("X-RPCX-ServicePath", servicePath)
  req.Header.Add("X-RPCX-ServiceMethod", serviceMethod)
  req.Header.Add("X-RPCX-SerializeType", "1")

  rsp, _ := clientx.Do(req)
  logs.Debugf("http2 RPC response -->>> %+v", rsp)
  if rsp == nil {
    logs.Errorf("-->>> http2 service %+v, %+v not response anything",
      servicePath, client.Http2SvcNode)
    return nil
  }
  defer rsp.Body.Close()

  data, _ := ioutil.ReadAll(rsp.Body)
  logs.Debugf("http2 RPC response reply -->>> %+v", string(data))
  err = json.Unmarshal([]byte(data), reply)
  return err
}

func (client *Client) Http2CallGw(ctx context.Context, servicePath, serviceMethod string, args interface{},
  reply interface{}) error {
  logs.Debugf("=== call Http2Service Http2CallGw ===")

  //t1 := &http.Transport{
  //  ExpectContinueTimeout: 3*time.Second, // for example
  //}
  //http2.ConfigureTransport(t1)

  clientx := http.Client{
    Transport:     &http2.Transport{
      AllowHTTP:  true,
      DialTLS: func(network, addr string, cfg *tls.Config) (conn net.Conn, err error) {
        return net.Dial(network, addr)
      },
    },
    Timeout:  3 * time.Second,
  }

  //timeout := time.Duration(Http2CallTimeout) * time.Second
  //clientx.Timeout = timeout
  //url := "http://127.0.0.1:8972"
  url := fmt.Sprintf("http://%s", client.Http2SvcNode)
  //payload, err := json.Marshal(args)
  payload := args.([]byte)
  //var err error
  //logs.ErrorfCheck(err, "http2 client serialize payload error, args -->>> %+v", args)

  req, _ := http.NewRequest("POST", url, bytes.NewBuffer(payload))
  req.Header.Add("X-RPCX-ServicePath", servicePath)
  req.Header.Add("X-RPCX-ServiceMethod", serviceMethod)
  req.Header.Add("X-RPCX-SerializeType", "1")

  rsp, _ := clientx.Do(req)
  logs.Debugf("gateway http2 RPC response -->>> %+v", rsp)
  if rsp == nil {
    logs.Errorf("-->>> http2 service %+v, %+v not response anything",
      servicePath, client.Http2SvcNode)
    return nil
  }
  defer rsp.Body.Close()

  data, _ := ioutil.ReadAll(rsp.Body)
  logs.Debugf("gateway http2 RPC response reply -->>> %+v", string(data))
  err := json.Unmarshal(data, reply)
  return err
}

// Go invokes the function asynchronously. It returns the Call structure representing
// the invocation. The done channel will signal when the call is complete by returning
// the same Call object. If done is nil, Go will allocate a new channel.
// If non-nil, done must be buffered or Go will deliberately crash.
func (client *Client) Go(ctx context.Context, servicePath, serviceMethod string, args interface{}, reply interface{}, done chan *Call) *Call {
  call := new(Call)
  call.ServicePath = servicePath
  call.ServiceMethod = serviceMethod
  meta := ctx.Value(share.ReqMetaDataKey)
  if meta != nil { //copy meta in context to meta in requests
    call.Metadata = meta.(map[string]string)
  }

  if _, ok := ctx.(*share.Context); !ok {
    ctx = share.NewContext(ctx)
  }

  // TODO: should implement as plugin
  client.injectOpenTracingSpan(ctx, call)
  client.injectOpenCensusSpan(ctx, call)

  call.Args = args
  call.Reply = reply      // todo: 这里已经初始化了reply的内存
  logs.Debugf("call.Reply 1 -------------- %+v", call.Reply)

  if done == nil {
    logs.Debugf("赋值给call.Doone 的 done 为空 ---- %+v", &done)
    done = make(chan *Call, 10) // buffered.
  } else {
    logs.Debugf("赋值给call.Doone 的 done 不为空 ------- %+v", &done)
    // If caller passes done != nil, it must arrange that
    // done has enough buffer for the number of simultaneous
    // RPCs that will be using that channel. If the channel
    // is totally unbuffered, it's best not to run at all.
    if cap(done) == 0 {
      logs.Panic("rpc: done channel is unbuffered")
    }
  }
  call.Done = done
  logs.Debug("call 4: %+v ==> %+v ==> %+v ==> %+v \n <===== one client call done =====>\n\n", reflect.TypeOf(call), call, call.Args, call.Reply)
  logs.Debugf("client.send之前 call.Done --------- %+v", call.Done)

  client.send(ctx, call)        // todo: 发送客户端请求给服务端

  logs.Debug("client.send之后 call.Done --------- %+v", call.Done)
  logs.Debug("client.send 之后  call.Done channel就会写进数据, 然后就不会阻塞")
  logs.Debugf("假如这个print比call 3的 select case后执行，则reply没数据，因为print输出调用call的时候，call已经被case从channel接收走了，%+v call 5: %+v ==> %+v ==> %+v ==> %+v \n <===== one client call done =====>\n\n", time.Now(), reflect.TypeOf(call), call, call.Args, call.Reply)
  return call
}

func (client *Client) injectOpenTracingSpan(ctx context.Context, call *Call) {
  var rpcxContext *share.Context
  var ok bool
  if rpcxContext, ok = ctx.(*share.Context); !ok {
    return
  }
  sp := rpcxContext.Value(share.OpentracingSpanClientKey)
  if sp == nil { // have not config opentracing plugin
    return
  }

  span := sp.(opentracing.Span)
  if call.Metadata == nil {
    call.Metadata = make(map[string]string)
  }
  meta := call.Metadata

  err := opentracing.GlobalTracer().Inject(
    span.Context(),
    opentracing.TextMap,
    opentracing.TextMapCarrier(meta))
  if err != nil {
    logs.Errorf("failed to inject span: %v", err)
  }
}

func (client *Client) injectOpenCensusSpan(ctx context.Context, call *Call) {
  var rpcxContext *share.Context
  var ok bool
  if rpcxContext, ok = ctx.(*share.Context); !ok {
    return
  }
  sp := rpcxContext.Value(share.OpencensusSpanClientKey)
  if sp == nil { // have not config opencensus plugin
    return
  }

  span := sp.(*trace.Span)
  if span == nil {
    return
  }
  if call.Metadata == nil {
    call.Metadata = make(map[string]string)
  }
  meta := call.Metadata

  spanContext := span.SpanContext()
  scData := make([]byte, 24)
  copy(scData[:16], spanContext.TraceID[:])
  copy(scData[16:24], spanContext.SpanID[:])
  meta[share.OpencensusSpanRequestKey] = string(scData)
}

// Call invokes the named function, waits for it to complete, and returns its error status.
func (client *Client) Call(ctx context.Context, servicePath, serviceMethod string, args interface{}, reply interface{}) error {
  // todo: http2 client call add here
  if client.option.Http2 {
    	return client.Http2Call(ctx, servicePath, serviceMethod, args, reply)
  }

  return client.call(ctx, servicePath, serviceMethod, args, reply)
}

func (client *Client) call(ctx context.Context, servicePath, serviceMethod string, args interface{}, reply interface{}) error {
  seq := new(uint64)
  ctx = context.WithValue(ctx, seqKey{}, seq)
  Done := client.Go(ctx, servicePath, serviceMethod, args, reply, make(chan *Call, 1)).Done
  logs.Debugf("call Go Done, 如果客户端收不到服务端的数据，这里会一直阻塞, 客户端收到服务端数据才会写进Done chann *call: %+v", Done)

  var err error
  select {
  case <-ctx.Done(): //cancel by context
    client.mutex.Lock()
    call := client.pending[*seq]
    delete(client.pending, *seq)
    client.mutex.Unlock()
    if call != nil {
      call.Error = ctx.Err()
      call.done()
    }

    return ctx.Err()

  case call := <-Done:      // 当前的call已经完成
    logs.Debug("从time.Now()得知这里是最后获取数据的 %+v call 3: %+v ==> %+v ==> %+v ==> %+v \n <===== one client call done =====>\n\n", time.Now(), reflect.TypeOf(call), call, call.Args, call.Reply)
    err = call.Error
    meta := ctx.Value(share.ResMetaDataKey)
    if meta != nil && len(call.ResMetadata) > 0 {
      resMeta := meta.(map[string]string)
      for k, v := range call.ResMetadata {
        resMeta[k] = v
      }
    }
  }

  return err
}

// SendRaw sends raw messages. You don't care args and replys.
func (client *Client) SendRaw(ctx context.Context, r *protocol.Message) (map[string]string, []byte, error) {
  logs.Debugf("-->>> gateway call SendRaw...")

  if client.option.Http2 {
   return client.Http2CallSendRaw(ctx, r)
  }

  ctx = context.WithValue(ctx, seqKey{}, r.Seq())
  call := new(Call)
  call.Raw = true     // todo: 定义Raw为true， 服务端不会进行对数据的序列化
  call.ServicePath = r.ServicePath
  call.ServiceMethod = r.ServiceMethod
  meta := ctx.Value(share.ReqMetaDataKey)

  rmeta := make(map[string]string)

  // copy meta to rmeta
  if meta != nil {
    for k, v := range meta.(map[string]string) {
      rmeta[k] = v
    }
  }
  // copy r.Metadata to rmeta
  if r.Metadata != nil {
    for k, v := range r.Metadata {
      rmeta[k] = v
    }
  }

  if meta != nil { //copy meta in context to meta in requests
    call.Metadata = rmeta
  }
  r.Metadata = rmeta

  // fixme: done的channel长度只有10， 可能这里是一个性能瓶颈
  done := make(chan *Call, 10)
  //logs.Debugff("done 1 ----------------- %+v", done)
  call.Done = done          // todo： 某个gor改变call.Done 从而改变done, 可能是Go函数?
  //logs.Debugff("call.Done 1 ----------------- %+v", call.Done)
  //logs.Debugff("done 2 ----------------- %+v", done)

  // todo: 转化XMessageIDVal的值
  seq := r.Seq()
  //logs.Printf("seq XMessageID ----------------- %+v", seq)

  client.mutex.Lock()
  if client.pending == nil {
    client.pending = make(map[uint64]*Call)
  }
  client.pending[seq] = call        // todo: 通过这里传入call， 有协程在监听pending，然后改变call的状态
  //logs.Debugff("done 3 ----------------- %+v", done)
  client.mutex.Unlock()

  // todo: 这里是压缩， 不是用序列化方法， 假如采用加密方式， Encode 会加密请求的数据
  logs.Debugf("Client.SendRaw request payload send server -->>> %+v", string(r.Payload))
  data := r.Encode() // 请求的所有数据转化为[]byte
  // todo: 一共有两个 client.Conn.Write(data) 执行，一个是在SenfRaw 给gateway 调用, 一个是在 Client.Send() 给常规的client调用
  // todo: 返回服务端结果的关键就是这个 client.Conn.Write(data), 关键是在client执行Connect的时候，其中的 c.r = bufio.NewReaderSize(conn, ReaderBuffsize)
  // todo: 会实时从conn中读取server的返回，然后写入c.r，再通过 go.c.inpunt() 去读取c.r, 最后写入call.reply, 完成整个client到server的调用
  _, err := client.Conn.Write(data)   // todo: if the service is only http2, here will panic, because the client.Conn is nil
  //logs.Debug("client.Conn.Write err -----------------", err)
  //logs.Debugff("done 4 ----------------- %+v", done)
  //logs.Debugff("call.Done 2 ----------------- %+v", call.Done)

  if err != nil {
    logs.Debug("client.Conn.Write(data) err -------", err)
    client.mutex.Lock()
    call = client.pending[seq]
    delete(client.pending, seq)
    client.mutex.Unlock()
    if call != nil {
      call.Error = err
      call.done()
    }
    return nil, nil, err
  }

  if r.IsOneway() {
    logs.Debug("---@@@------- IsOneway --------@@@---")
    client.mutex.Lock()
    call = client.pending[seq]
    delete(client.pending, seq)
    client.mutex.Unlock()
    if call != nil {
      call.done()
    }
    return nil, nil, nil
  }

  var m map[string]string
  var payload []byte

  select {
  case <-ctx.Done(): //cancel by context
    logs.Debug("---@@@------- ctx.Done() --------@@@---")
    client.mutex.Lock()
    call := client.pending[seq]
    delete(client.pending, seq)
    client.mutex.Unlock()
    if call != nil {
      call.Error = ctx.Err()
      call.done()
    }
    return nil, nil, ctx.Err()

  case call := <-done:        // todo: 写入done channel的就是这个call请求本身
    logs.Debugf("---@@@------- <-done --------@@@--- %+v", done)
    //logs.Debugff("select call := <-done  %+v ----------------", call)
    err = call.Error
    m = call.Metadata
    logs.Debugf("Client.SendRaw 中得到的 m ----------- %+v, %+v", len(m), m)
    if call.Reply != nil {
      logs.Debug("----@@----call.Reply != nil ----@@----")
      payload = call.Reply.([]byte)
    }

  //default:
  //logs.Debugf("done 5 ----------------- %+v", done)
  }

  // fixme: 设计缺陷， 这里return的根本就不是处理之后的m, payload， 因为假如是采用了加密方式的话， 这里的数据已经变了
  return m, payload, err
}

func (client *Client) Http2CallSendRaw(ctx context.Context, r *protocol.Message) (map[string]string, []byte, error) {
  logs.Debugf("-->>> gateway call Http2CallSendRaw")
  servicePath := r.ServicePath
  ServiceMethod := r.ServiceMethod
  var reply interface{}

  logs.Debugf("before Http2CallSendRaw Http2Call reply -->>> %+v", reply)
  client.Http2CallGw(ctx, servicePath, ServiceMethod, r.Payload, &reply)
  //payload := map[string]string {
  // "name": "halokid",
  //}
  //client.Http2Call(ctx, servicePath, ServiceMethod, payload, &reply)

  logs.Debugf("after Http2CallSendRaw Http2Call reply -->>> %+v", reply)
  replyx, _ := json.Marshal(reply)

  if reply == nil {
    return nil, replyx, errors.New("Http2CallSendRaw Http2Call reply is nil")
  }

  return nil, replyx, nil
}

func convertRes2Raw(res *protocol.Message) (map[string]string, []byte, error) {
  //logs.Debugf("res.Payload 1 ---------------- %+v", res.Payload)
  m := make(map[string]string)
  m[XVersion] = strconv.Itoa(int(res.Version()))
  if res.IsHeartbeat() {
    m[XHeartbeat] = "true"
  }
  if res.IsOneway() {
    m[XOneway] = "true"
  }
  if res.MessageStatusType() == protocol.Error {
    m[XMessageStatusType] = "Error"
  } else {
    m[XMessageStatusType] = "Normal"
  }

  if res.CompressType() == protocol.Gzip {
    m["Content-Encoding"] = "gzip"
  }

  //logs.Debugf("res.Payload 2 ---------------- %+v", res.Payload)

  m[XMeta] = urlencode(res.Metadata)
  m[XSerializeType] = strconv.Itoa(int(res.SerializeType()))
  m[XMessageID] = strconv.FormatUint(res.Seq(), 10)
  m[XServicePath] = res.ServicePath
  m[XServiceMethod] = res.ServiceMethod

  //logs.Debugf("res.Payload 3 ---------------- %+v", res.Payload)

  return m, res.Payload, nil
}

func urlencode(data map[string]string) string {
  if len(data) == 0 {
    return ""
  }
  var buf bytes.Buffer
  for k, v := range data {
    buf.WriteString(url.QueryEscape(k))
    buf.WriteByte('=')
    buf.WriteString(url.QueryEscape(v))
    buf.WriteByte('&')
  }
  s := buf.String()
  return s[0 : len(s)-1]
}

func (client *Client) send(ctx context.Context, call *Call) {

  // Register this call.
  // client.Conn.Write(data) 发送数据给服务端
  logs.Debugf("call 1: %+v ==> %+v ==> %+v ==> %+v \n <===== one client call done =====>\n\n", reflect.TypeOf(call), call, call.Args, call.Reply)

  client.mutex.Lock()       // 锁住client， mutex作为一个锁句柄，放在client里面作为属性，方便调用
  if client.shutdown || client.closing {
    call.Error = ErrShutdown
    client.mutex.Unlock()
    call.done()
    return
  }

  // todo: 数据序列化方式作为key传给 share.Codes这个map, 获取不同的序列化配适器
  codec := share.Codecs[client.option.SerializeType]
  if codec == nil {
    call.Error = ErrUnsupportedCodec
    client.mutex.Unlock()
    call.done()
    return
  }

  if client.pending == nil {
    client.pending = make(map[uint64]*Call)
  }

  seq := client.seq
  client.seq++
  client.pending[seq] = call        // todo: client正在处理的是当前的call, 把call加入client的pending列表
  client.mutex.Unlock()

  if cseq, ok := ctx.Value(seqKey{}).(*uint64); ok {
    *cseq = seq
  }

  //req := protocol.NewMessage()
  req := protocol.GetPooledMsg()
  req.SetMessageType(protocol.Request)    // 设置数据类型是request
  req.SetSeq(seq)
  if call.Reply == nil {
    logs.Debug("call.Reply is nil, 不需要服务端返回 run here!!! -------------")
    req.SetOneway(true)
  }

  // heartbeat
  if call.ServicePath == "" && call.ServiceMethod == "" {
    req.SetHeartbeat(true)
  } else {
    req.SetSerializeType(client.option.SerializeType)
    if call.Metadata != nil {
      req.Metadata = call.Metadata
    }

    req.ServicePath = call.ServicePath
    req.ServiceMethod = call.ServiceMethod

    data, err := codec.Encode(call.Args)
    if err != nil {
      delete(client.pending, seq)
      call.Error = err
      call.done()
      return
    }
    if len(data) > 1024 && client.option.CompressType != protocol.None {
      // 加入byte的长度大于1024才需要压缩
      req.SetCompressType(client.option.CompressType)
    }

    req.Payload = data
  }

  if client.Plugins != nil {
    client.Plugins.DoClientBeforeEncode(req)
  }
  data := req.Encode()

  logs.Debugf("%+v call 6: %+v ==> %+v ==> %+v ==> %+v \n <===== one client call done =====>\n\n", time.Now(), reflect.TypeOf(call), call, call.Args, call.Reply)

  // todo: 发送给服务端之后，客户端是怎样获取返回的数据的呢？
  // todo: 关键点1:  client 声明的 service reply结构体的数据是在 client.call 的流程中更改的, 也就是写入了服务端的返回数据， 而不是等 client.call 整个流程完成之后才写入的, client 监听 chan 作为 call流程的完成方式
  // todo: 整体通信机制流程梳理
  // todo: 客户端选择哪个服务端节点的关键是在 k := c.selector.Select(ctx, servicePath, serviceMethod, args), 这里会使用一些选择的算法去进行服务端节点的选择，逻辑在函数selectClient中
  // todo: 1. client.Conn 和服务端建立连接，代码xclient.go 的 getCachedClient 函数, 这个函数调用了 client/connection.go 的 Connect 函数, xClient在调用 Call 时触发 c.selectClient, 才会触发客户端跟服务端执行真正的网络连接， NewXClientPool 这个方法建立的pool仅仅只是一个对象重用的pool， 并不是真正的连接池
  // todo: 2. 1建立连接之后， Connect函数的 c.r = bufio.NewReaderSize(conn, ReaderBuffsize)，会一直监听客户端和服务端连接的数据交互，只要服务端往客户端写数据， c.r就能获取到数据
  // todo: 3.  Connect函数的 go c.input() 一直在监听 client 的 接收数据， 处理接收数据， 然后把完成之后的call对象，写入本身这个call的 call.Done 属性, 然后client 的 call函数的 case call := <-Done: 就能获取chan 的输入， 完成整个client.call的流程
  // todo: 4.  那究竟是怎么把服务端的返回写入 call.Reply的呢？ 在 c.input函数里面， 通过 call = client.pending[seq]  取得每一次的call对象， 再通过 err = codec.Decode(data, call.Reply)，赋值给call.Reply, 然后因为开始客户端定义 reply := &Reply{},  是一个引用指针， 当 call.Reply = reply的时候， call.Reply 承接了这个指针， 当改变  call.Reply 的值时， 就会改变 &Reply{}, 就会改变 reply， 所以客户端可以用reply来捕获服务端的输出, 具体的范例在 testWriteToReply
  _, err := client.Conn.Write(data)   // todo: 向连接服务端的conn写入数据

  logs.Debugf("%+v call 7: %+v ==> %+v ==> %+v ==> %+v \n <===== one client call done =====>\n\n", time.Now(), reflect.TypeOf(call), call, call.Args, call.Reply)

  //logs.Printf("client send data to serv: %+v -- %+v -- %+v", reflect.TypeOf(data), data, string(data[0:]))
  logs.Debugf("client send data to serv: %+v ==> %+v ==> %+v", reflect.TypeOf(data), data, *(*string)(unsafe.Pointer(&data)))
  logs.Debugf("%+v", *(*string)(unsafe.Pointer(&data)))
  logs.Debugf(" --  " + string(data[:]) + "\n")
  logs.Debugf("头16位是:--- " + string(data[:16]) + "\n")

  logs.Debugf("%+v call 8: %+v ==> %+v ==> %+v ==> %+v \n <===== one client call done =====>\n\n", time.Now(), reflect.TypeOf(call), call, call.Args, call.Reply)

  ///**
  // todo: 如果client 跟 server在通信过程中有错误，则从client 的 pending 列表里面delete这个有错误的call， 设置call.Error 和 call.done()，直接返回
  if err != nil {
    client.mutex.Lock()
    call = client.pending[seq]
    delete(client.pending, seq)
    client.mutex.Unlock()
    if call != nil {
      call.Error = err
      call.done()
    }
    protocol.FreeMsg(req)
    return
  }
  //*/

  isOneway := req.IsOneway()
  protocol.FreeMsg(req)

  // todo: 输出call的整体数据， 这里因为call 是一个 chan， 可能被其他协程的select监听输出了，所以这里输出的call是一个初始化的call，还没有赋值reply的都有可能，所以这里不应该这样输出， 会误导调试数据逻辑
  logs.Debugf("%+v call 2: %+v ==> %+v ==> %+v ==> %+v \n <===== one client call done =====>\n\n", time.Now(), reflect.TypeOf(call), call, call.Args, call.Reply)

  if isOneway {
    logs.Debugf(" ======= after call isOneway =======")
    client.mutex.Lock()
    call = client.pending[seq]
    delete(client.pending, seq)
    client.mutex.Unlock()
    if call != nil {
      call.done()
    }
  }

  if client.option.WriteTimeout != 0 {
    client.Conn.SetWriteDeadline(time.Now().Add(client.option.WriteTimeout))
  }

}

func (client *Client) input() {
  var err error

  for err == nil {
    var res = protocol.NewMessage()
    if client.option.ReadTimeout != 0 {
      client.Conn.SetReadDeadline(time.Now().Add(client.option.ReadTimeout))
    }


    //logs.Debugf("res.Payload 1 --------------------- %+v", res.Payload)
    //logs.Debugf("len: client.r 2 ---------------------  %+v", client.r)
    //logs.Debugf("res.data 1 ---------------------  %+v", res)
    // todo: 是input改变了 client.r 的值???,  Decode函数一直在读取 client.r的数据
    // todo: res 就是服务端执行之后，返回给客户端的数据
    // todo: 包含解压的过程
    logs.Debugf(" ========= res.MessageStatusType() 1 ======== %+v", res.MessageStatusType())

    // todo: 包含序列化数据的过程， 如果客户端和服务端序列化方法不同， 会把产生的错误写进  res.MessageStatusType(). 如果是服务端检查到 序列化方法不对， 会返回给  client.r, 具体是会把 protol.Error 定义为1
    err = res.Decode(client.r)
    logs.Debugf(" ========= res.MessageStatusType() 2 ======== %+v", res.MessageStatusType())
    //logs.Debugf("res.Payload 2 --------------------- %+v", res.Payload)

    if err != nil {
      break
    }
    if client.Plugins != nil {
      client.Plugins.DoClientAfterDecode(res)
    }

    seq := res.Seq()
    var call *Call
    isServerMessage := (res.MessageType() == protocol.Request && !res.IsHeartbeat() && res.IsOneway())
    if !isServerMessage {
      client.mutex.Lock()
      // fixme: 如果这个seq是一样的， 会不会不同的处理请求，取得了同一个call？？gateway现在的sep是一样的， 这个要验证下， 如果取得了同一个call的话，可能会产生 call对象错乱的问题， 而造成数据错误, 目前client/serqver模式没有这个问题
      // todo: 通过这里取得call对象，然后把 服务端的返回写入 call.Reply
      call = client.pending[seq]      // todo: 取得在SendRaw的时候写入的pending call
      delete(client.pending, seq)
      client.mutex.Unlock()
    }

    //logs.Debugf("call.Reply 1 --------------------- %+v", call.Reply)

    switch {    // todo: 协程重复执行 input()， 这个switch逻辑一会一直监听执行
    case call == nil:
      logs.Debugf(" ========= go client.input() 1, call is nil ========")
      if isServerMessage {
        if client.ServerMessageChan != nil {
          go client.handleServerRequest(res)
        }
        continue
      }

    case res.MessageStatusType() == protocol.Error:
      // We've got an error response. Give this to the request
      logs.Debugf(" ========= 序列化数据错误 go client.input() 2, client msg protocol Error ========")
      if len(res.Metadata) > 0 {
        call.ResMetadata = res.Metadata
        call.Error = ServiceError(res.Metadata[protocol.ServiceError])
      }

      if call.Raw {
        call.Metadata, call.Reply, _ = convertRes2Raw(res)
        call.Metadata[XErrorMessage] = call.Error.Error()
      }
      call.done()

    default:
      logs.Debugf(" ========= go client.input() 3, all is fine ========")
      if call.Raw {
        //logs.Debugf("res.Payload 3 --------------------- %+v", res.Payload)
        //logs.Debugf("call.Reply 2 --------------------- %+v", call.Reply)
        // todo: 重点， 假如是GW的请求， 根本就不会走 解压缩的逻辑， 因为GW调用 SendRaw 方法， 定义了 Call.RAW 为 true
        call.Metadata, call.Reply, _ = convertRes2Raw(res)
        //logs.Debugf("call.Reply 3 --------------------- %+v", string(call.Reply.([]uint8)))
      } else {
        data := res.Payload
        if len(data) > 0 {
          codec := share.Codecs[res.SerializeType()]
          if codec == nil {
            call.Error = ServiceError(ErrUnsupportedCodec.Error())
          } else {
            // todo: 把服务端的处理结果写入 call.Reply, 从而影响 reply = &Reply{}, 具体范例在echo client sample 的 testWriteToReply
            logs.Debugf("data Decode --------- %+v, %+v, %+v", reflect.TypeOf(data), len(data), string(data))
            logs.Debugf("call.Reply Decode --------- %+v, %+v", reflect.TypeOf(call.Reply), call.Reply)
            err = codec.Decode(data, call.Reply)
            if err != nil {
              call.Error = ServiceError(err.Error())
            }
          }
        }
        if len(res.Metadata) > 0 {
          call.ResMetadata = res.Metadata
        }

      }

      call.done()       // todo: 更改call状态的逻辑, 把数据写入 call.Done 这个 chann, 触发 client.go 的 case call := <-Done 这段逻辑， 代表client的call完成
    }
  }
  // Terminate pending calls.

  if client.ServerMessageChan != nil {
    //logs.Debugf("---@@@@@------client.ServerMessageChan != nil ---@@@@@-----")
    req := protocol.NewMessage()
    req.SetMessageType(protocol.Request)
    req.SetMessageStatusType(protocol.Error)
    if req.Metadata == nil {
      req.Metadata = make(map[string]string)
      if err != nil {
        req.Metadata[protocol.ServiceError] = err.Error()
      }
    }
    req.Metadata["server"] = client.Conn.RemoteAddr().String()
    go client.handleServerRequest(req)
  }

  client.mutex.Lock()
  if !client.pluginClosed {
    if client.Plugins != nil {
      client.Plugins.DoClientConnectionClose(client.Conn)
    }
    client.pluginClosed = true
  }

  client.Conn.Close()         // todo: client关闭与服务端的连接
  client.shutdown = true
  closing := client.closing
  if err == io.EOF {
    if closing {
      err = ErrShutdown
    } else {
      err = io.ErrUnexpectedEOF
    }
  }
  for _, call := range client.pending {
    call.Error = err
    logs.Debug("客户端调用超时错误，客户端会关闭连接: ---------- ", err)
    call.done()
  }

  client.mutex.Unlock()

  if err != nil && err != io.EOF && !closing {
    logs.Error("rpcx: client protocol error:", err)
  }
}

func (client *Client) handleServerRequest(msg *protocol.Message) {
  defer func() {
    if r := recover(); r != nil {
      logs.Errorf("ServerMessageChan may be closed so client remove it. Please add it again if you want to handle server requests. error is %v", r)
      client.ServerMessageChan = nil
    }
  }()

  t := time.NewTimer(5 * time.Second)
  select {
  case client.ServerMessageChan <- msg:
  case <-t.C:
    logs.Warnf("ServerMessageChan may be full so the server request %d has been dropped", msg.Seq())
  }
  t.Stop()
}

func (client *Client) heartbeat() {
  t := time.NewTicker(client.option.HeartbeatInterval)

  for range t.C {
    if client.IsShutdown() || client.IsClosing() {
      t.Stop()
      return
    }

    err := client.Call(context.Background(), "", "", nil, nil)
    if err != nil {
      logs.Warnf("failed to heartbeat to %s", client.Conn.RemoteAddr().String())
    }
  }
}

// Close calls the underlying connection's Close method. If the connection is already
// shutting down, ErrShutdown is returned.
func (client *Client) Close() error {
  client.mutex.Lock()

  for seq, call := range client.pending {
    delete(client.pending, seq)
    if call != nil {
      call.Error = ErrShutdown
      call.done()
    }
  }

  var err error
  if !client.pluginClosed {
    if client.Plugins != nil {
      client.Plugins.DoClientConnectionClose(client.Conn)
    }

    client.pluginClosed = true
    //err = client.Conn.Close()

    if !client.option.Http2 {
      err = client.Conn.Close()
    }
  }

  if client.closing || client.shutdown {
    client.mutex.Unlock()
    return ErrShutdown
  }

  client.closing = true
  client.mutex.Unlock()
  return err
}
