package client
/*
设计思路:
XClient interface 封装所有对外的方法， 可外部调用
xClient struct 定义为内部struct， imple方法并定义属性， 只能内部调用
 */
import (
	"bufio"
	"context"
	"errors"
	"fmt"
	logs "github.com/halokid/rpcx-plus/log"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	ex "github.com/halokid/rpcx-plus/errors"
	"github.com/halokid/rpcx-plus/protocol"
	"github.com/halokid/rpcx-plus/serverplugin"
	"github.com/halokid/rpcx-plus/share"
	"github.com/juju/ratelimit"
	//. "github.com/halokid/ColorfulRabbit"

	"github.com/halokid/ColorfulRabbit"
	"github.com/mozillazg/request"
)

const (
	FileTransferBufferSize = 1024
)

var (
	// ErrXClientShutdown xclient is shutdown.
	ErrXClientShutdown = errors.New("xClient is shut down")
	// ErrXClientNoServer selector can't found one server.
	ErrXClientNoServer = errors.New("服务不可用或没有相应的服务名----can not found any server")
	// ErrServerUnavailable selected server is unavailable.
	ErrServerUnavailable = errors.New("selected server is unavilable")
)

// XClient is an interface that used by client with service discovery and service governance.
// One XClient is used only for one service. You should create multiple XClient for multiple services.
type XClient interface {
	SetPlugins(plugins PluginContainer)
	GetPlugins() PluginContainer
	SetSelector(s Selector)
	ConfigGeoSelector(latitude, longitude float64)
	Auth(auth string)

	Go(ctx context.Context, serviceMethod string, args interface{}, reply interface{}, done chan *Call) (*Call, error)
	Call(ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error
  CallNotGo(svc string, md string, pairs []*KVPair) string
	Broadcast(ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error
	Fork(ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error
	SendRaw(ctx context.Context, r *protocol.Message) (map[string]string, []byte, error)
	SendFile(ctx context.Context, fileName string, rateInBytesPerSecond int64) error
	DownloadFile(ctx context.Context, requestFileName string, saveTo io.Writer) error
	Close() error

	// 非go语言
	IsGo() bool
	GetNotGoServers() map[string]string
	GetSvcTyp() string

	GetReverseProxy()	bool
	SelectNode(ctx context.Context, servicePath, serviceMethod string, args interface{}) string
}

// KVPair contains a key and a string.
type KVPair struct {
	Key   string
	Value string
}

// ServiceDiscoveryFilter can be used to filter services with customized logics.
// Servers can register its services but clients can use the customized filter to select some services.
// It returns true if ServiceDiscovery wants to use this service, otherwise it returns false.
type ServiceDiscoveryFilter func(kvp *KVPair) bool

// ServiceDiscovery defines ServiceDiscovery of zookeeper, etcd and consul
type ServiceDiscovery interface {
	// todo: 每种服务注册的方式都要继承这个interface的方法
	GetServices() []*KVPair
	WatchService() chan []*KVPair
	RemoveWatcher(ch chan []*KVPair)
	Clone(servicePath string) ServiceDiscovery
	SetFilter(ServiceDiscoveryFilter)
	Close()
}

type xClient struct {
	failMode     FailMode
	selectMode   SelectMode
	cachedClient map[string]RPCClient
	breakers     sync.Map
	servicePath  string
	option       Option

	mu        sync.RWMutex
	servers   map[string]string
	discovery ServiceDiscovery
	selector  Selector

	isShutdown bool

	// auth is a string for Authentication, for example, "Bearer mF_9.B5f-4.1JqM"
	auth string

	Plugins PluginContainer

	ch chan []*KVPair

	serverMessageChan chan<- *protocol.Message
	
	isGo bool			// 非go服务端的标识
	svcTyp   string
	notGoServers map[string]string			// 非go服务端的地址

	isReverseProxy 		bool
}

// NewXClient creates a XClient that supports service discovery and service governance.
func NewXClient(servicePath string, failMode FailMode, selectMode SelectMode, discovery ServiceDiscovery, option Option) XClient {
	client := &xClient{
		failMode:     failMode,
		selectMode:   selectMode,
		discovery:    discovery,
		servicePath:  servicePath,
		cachedClient: make(map[string]RPCClient),
		option:       option,
	}
	client.isGo = true

	// todo: 初次获取服务节点列表数据
	pairs := discovery.GetServices()
	servers := make(map[string]string, len(pairs))
	/**
	kCk := ""
	for i, p := range pairs {
		if i == 0 {
			kCk = p.Key	
		}
		servers[p.Key] = p.Value
	}
	*/
	// todo: 生产服务节点信息slice
	for _, p := range pairs {
		servers[p.Key] = p.Value
	}
	// todo: 过滤禁用了的服务节点
	filterByStateAndGroup(client.option.Group, servers)

	// todo: 定义servers， 修复相似服务名bug在这里
	client.servers = servers
	logs.Debugf("NewXClient建立初次获取client.servers --------------- %+v", client.servers)
	
	// 检查第一个key属于什么typ
	//serCk := servers[kCk]
	//typ := GetSpIdx(serCk, "&", -1)
	//if typ == "typ=py" {
		// python服务端
		//return "pyTyp"
	//}
	
	//logs.Debug("找到的servers:", servers)
	/*
	isNotGoSvc := []string{"typ=py", "typ=rust"}
	for _, v := range servers {
		//if strings.Index(v, "typ=py") != -1 {
		if ColorfulRabbit.InSlice(v, isNotGoSvc) {	
			// 为非go语言
			client.isGo = false 
			client.notGoServers = servers
			break
		} 
	}
	*/

	// todo: handler here, we separate NotGoSvc(includes py, rust, CakeRabbit) and proxy-service
	// todo: but after that, we found rust and cake should use http default, so this is the same
	// todo: as isReverseProxy handle. so remove rust and cake check in genNotGo
	genIsReverseProxy(client, servers)
	if !client.isReverseProxy {			// if reverse proxy, dont need to genNotGo svc nodes
		genNotGoSvc(client, servers)
	}
	//client.isReverseProxy = true		// todo: hack code the property to test tracing
	
	if selectMode != Closest && selectMode != SelectByUser {
		client.selector = newSelector(selectMode, servers)
	}

	client.Plugins = &pluginContainer{}

	// todo: discovery.WatchService() 返回的 ch 是一个 []*KVPair 指针
	ch := client.discovery.WatchService()
	logs.Debugf("观察svc变化 ch ----------- %+v", ch)
	if ch != nil {
		logs.Debug("--@@@----守护监听注册中心svc:", servicePath, "的变化----@@@--")
		client.ch = ch
		// todo: here just update the client servers, real watch register nodes data change is in
		// todo: func NewConsulDiscoveryStore
		go client.watch(ch)		// todo: client.watch 不断监听  d.chans 的指针句柄, func (d *ConsulDiscovery) watch() 不断监听服务node的改变去更新  d.chans， 形状node数据更新闭环
	}

	return client
}

// generate the node is reverse proxy or not
func genIsReverseProxy(client *xClient, servers map[string]string) error {
	for k := range servers {
		logs.Debugf("计算是否为反代服务-genIsReverseProxy server ------- %+v", k)
		// todo: if the service is http, then reverser proxy the service
		if strings.Contains(k, "http") {
			client.isReverseProxy = true
			break
		}
	}
	return nil
}

// generate the info for not go service
// todo: this process must the service has the node already, is when the client create, the
// todo: service has no nodes, after that add the node, the client can not generate the sercvice
// todo: is Go or not.
func genNotGoSvc(client *xClient, servers map[string]string) error {
	isNotGo := false
	for _, v := range servers {
		if strings.Index(v, "typ=py") != -1 {
			//client.isGo = false
			client.svcTyp = "py"
			isNotGo = true
			//client.notGoServers = servers
			//break
		}
		//else if strings.Index(v, "typ=rust") != -1 {
		//	client.svcTyp = "rust"
		//	isNotGo = true
		//} else if strings.Index(v, "typ=cakeRabbit") != -1 {
		//	client.svcTyp = "cakeRabbit"
		//	isNotGo = true
		//}

		if isNotGo {
			client.isGo = false
			client.notGoServers = servers
		}
		break
	}
	logs.Debugf("client genNotGoSvc -------------- %+v", client)
	return nil
}

// NewBidirectionalXClient creates a new xclient that can receive notifications from servers.
func NewBidirectionalXClient(servicePath string, failMode FailMode, selectMode SelectMode, discovery ServiceDiscovery, option Option, serverMessageChan chan<- *protocol.Message) XClient {
	client := &xClient{
		failMode:          failMode,
		selectMode:        selectMode,
		discovery:         discovery,
		servicePath:       servicePath,
		cachedClient:      make(map[string]RPCClient),
		option:            option,
		serverMessageChan: serverMessageChan,
	}

	pairs := discovery.GetServices()
	servers := make(map[string]string, len(pairs))
	for _, p := range pairs {
		servers[p.Key] = p.Value
	}
	filterByStateAndGroup(client.option.Group, servers)
	client.servers = servers
	if selectMode != Closest && selectMode != SelectByUser {
		client.selector = newSelector(selectMode, servers)
	}

	client.Plugins = &pluginContainer{}

	ch := client.discovery.WatchService()
	if ch != nil {
		client.ch = ch
		go client.watch(ch)
	}

	return client
}

// SetSelector sets customized selector by users.
func (c *xClient) SetSelector(s Selector) {
	c.mu.RLock()
	s.UpdateServer(c.servers)
	c.mu.RUnlock()

	c.selector = s
}

// SetPlugins sets client's plugins.
func (c *xClient) SetPlugins(plugins PluginContainer) {
	c.Plugins = plugins
}

func (c *xClient) GetPlugins() PluginContainer {
	return c.Plugins
}

// ConfigGeoSelector sets location of client's latitude and longitude,
// and use newGeoSelector.
func (c *xClient) ConfigGeoSelector(latitude, longitude float64) {
	c.selector = newGeoSelector(c.servers, latitude, longitude)
	c.selectMode = Closest
}

// Auth sets s token for Authentication.
func (c *xClient) Auth(auth string) {
	c.auth = auth
}

// watch changes of service and update cached clients.
// todo: just update c.servers, not watch register nodes data change here.
// todo: watch the nodes change, check node is proxy or not, and change the isReverseProxy property
func (c *xClient) watch(ch chan []*KVPair) {
	logs.Debugf("len ch --------------- %+v", len(ch))
	for pairs := range ch {

		for k, v := range pairs {
			logs.Debugf("pairs k, v ------- %+v, %+v", k, v)
		}

		servers := make(map[string]string, len(pairs))
		for _, p := range pairs {
			servers[p.Key] = p.Value
		}
		c.mu.Lock()
		filterByStateAndGroup(c.option.Group, servers)
		c.servers = servers

		//isReverseProxyTag := false
		for k, v := range servers {
			logs.Debugf("servers k, v ------- %+v, %+v", k, v)
			if !c.isReverseProxy && checkIsReverseProxy(v) {
				logs.Debugf("--- change service [%s] isReverseProxy to true ---", c.servicePath)
				c.isReverseProxy = true
			}
		}

		if c.selector != nil {
			c.selector.UpdateServer(servers)
		}

		c.mu.Unlock()
		logs.Debug("\n\r")
	}
}

func checkIsReverseProxy(val string) bool {
	matches := []string{"typ=java", "typ=cakeRabbit", "typ=rust"}
	for _, m := range matches {
		if strings.Contains(val, m) {
			return true
		}
	}
	return false
}

func filterByStateAndGroup(group string, servers map[string]string) {
	for k, v := range servers {
		if values, err := url.ParseQuery(v); err == nil {
			if state := values.Get("state"); state == "inactive" {
				delete(servers, k)
			}
			if group != "" && group != values.Get("group") {
				delete(servers, k)
			}
		}
	}
}

// selects a client from candidates base on c.selectMode
func (c *xClient) selectClient(ctx context.Context, servicePath, serviceMethod string, args interface{}) (string, RPCClient, error) {
	//logs.Debug("selectClient ---------------")
	//logs.Debug("servers ---------------", c.servers)
	c.mu.Lock()
	k := c.selector.Select(ctx, servicePath, serviceMethod, args)
	c.mu.Unlock()
	if k == "" {
		return "", nil, ErrXClientNoServer
	}
	client, err := c.getCachedClient(k)
	return k, client, err
}

// select a node addr with select stategry
func (c *xClient) SelectNode(ctx context.Context, servicePath, serviceMethod string, args interface{}) string {
	node := c.selector.Select(ctx, servicePath, serviceMethod, args)
	return node
}

func (c *xClient) getCachedClient(k string) (RPCClient, error) {
	// TODO: improve the lock
	//logs.Debug("getCachedClient -----------------")
	// todo: 声明client为的接口类 RPCClient类型
	var client RPCClient
	var needCallPlugin bool
	c.mu.Lock()
	defer func() {
		if needCallPlugin {
			c.Plugins.DoClientConnected((client.(*Client)).Conn)
		}
	}()
	defer c.mu.Unlock()

	breaker, ok := c.breakers.Load(k)
	if ok && !breaker.(Breaker).Ready() {
		return nil, ErrBreakerOpen
	}

	client = c.cachedClient[k]
	//logs.Debug("c.cachedClient ----- @@@@@@@@@@@@@@---- ", c.cachedClient)
	if client != nil {
		if !client.IsClosing() && !client.IsShutdown() {
			return client, nil			// todo: 命中cacheClient则返回
		}
		logs.Debugf("client IsClosing() or IsShutdown(), client的k和状态 --- k: %+v," +
			" IsClosing(): %+v,  IsShutdown(): %+v", k, client.IsClosing(), client.IsShutdown())
		delete(c.cachedClient, k)
		client.Close()
	}

	// todo:  k不命中， 则client 为 nil, 这里更多的价值只是声明client类型
	client = c.cachedClient[k]
	if client == nil || client.IsShutdown() {
		network, addr := splitNetworkAndAddress(k)
		if network == "inprocess" {
			client = InprocessClient
		} else {
			client = &Client{			// todo: client本来是一个 RPCClient类型, xClient是从这里开始转变为client struct的，所以可以调用 client.conn
				option:  c.option,
				Plugins: c.Plugins,
			}

			var breaker interface{}
			if c.option.GenBreaker != nil {
				breaker, _ = c.breakers.LoadOrStore(k, c.option.GenBreaker())
			}
			//logs.Debug("getCache 11111111 --------------------------")
			// todo: client连接到server， 并且把连接句柄写入conn, 这是一个长连接，cache会一直保留这个连接
			// todo:  Connect函数会触发一个input的gor, go c.input() 这一句， 这个input就是更改 client.pending[seq]， 也就是网络请求连接状态的逻辑，SendRaw 和 call 都是靠这个来改变网络请求的状态

			// todo:  cachedClient有没有必要？cacheClient是一个以 service node的 address为key， client本身为value的 map，意思就是当某一个client要连某个service node 的时候，先在这个cacheCLient取client，这个在cache里面的client已经建立了客户端到服务端的conn， 所以不用再次建立这个conn，提高效率
			// todo: 关键性能点在于，当client 并发请求某 service node的时候， 这个cacheClient 里的client一直input一直在call service node，这里有一个for循环，不断的处理请求，当并发的时候，要for循环里面有swicth，就是当 协程重复执行 input()， 这个switch逻辑一会一直监听执行， 就是client就不会调用 conn.close()，这样就可以一直用已经建立了conn的client，提高性能， 当并发结束，则跳出switch, 按逻辑执行了 client.conn.close() 关闭客户端到服务端的连接.
			err := client.Connect(network, addr)
			logs.Debugf("完成client.Connect动作, err -------------- %+v", err)
			if err != nil {
				if breaker != nil {
					breaker.(Breaker).Fail()
				}
				return nil, err
			}
			if c.Plugins != nil {
				needCallPlugin = true
			}
		}

		client.RegisterServerMessageChan(c.serverMessageChan)

		c.cachedClient[k] = client
	}

	return client, nil
}

func (c *xClient) getCachedClientWithoutLock(k string) (RPCClient, error) {
	client := c.cachedClient[k]
	if client != nil {
		if !client.IsClosing() && !client.IsShutdown() {
			return client, nil
		}
		delete(c.cachedClient, k)
		client.Close()
	}

	//double check
	client = c.cachedClient[k]
	if client == nil || client.IsShutdown() {
		network, addr := splitNetworkAndAddress(k)
		if network == "inprocess" {
			client = InprocessClient
		} else {
			client = &Client{
				option:  c.option,
				Plugins: c.Plugins,
			}
			logs.Debug("getCache 22222 --------------------------")
			err := client.Connect(network, addr)
			if err != nil {
				return nil, err
			}
		}

		client.RegisterServerMessageChan(c.serverMessageChan)

		c.cachedClient[k] = client
	}

	return client, nil
}

func (c *xClient) removeClient(k string, client RPCClient) {
	c.mu.Lock()
	cl := c.cachedClient[k]
	if cl == client {
		delete(c.cachedClient, k)
	}
	c.mu.Unlock()

	if client != nil {
		client.UnregisterServerMessageChan()
		client.Close()
	}
}

func splitNetworkAndAddress(server string) (string, string) {
	ss := strings.SplitN(server, "@", 2)
	if len(ss) == 1 {
		return "tcp", server
	}

	return ss[0], ss[1]
}

// Go invokes the function asynchronously. It returns the Call structure representing the invocation. The done channel will signal when the call is complete by returning the same Call object. If done is nil, Go will allocate a new channel. If non-nil, done must be buffered or Go will deliberately crash.
// It does not use FailMode.
func (c *xClient) Go(ctx context.Context, serviceMethod string, args interface{}, reply interface{}, done chan *Call) (*Call, error) {
	if c.isShutdown {
		return nil, ErrXClientShutdown
	}

	if c.auth != "" {
		metadata := ctx.Value(share.ReqMetaDataKey)
		if metadata == nil {
			metadata = map[string]string{}
			ctx = context.WithValue(ctx, share.ReqMetaDataKey, metadata)
		}
		m := metadata.(map[string]string)
		m[share.AuthKey] = c.auth
	}

	_, client, err := c.selectClient(ctx, c.servicePath, serviceMethod, args)
	if err != nil {
		return nil, err
	}
	return client.Go(ctx, c.servicePath, serviceMethod, args, reply, done), nil
}

// Call invokes the named function, waits for it to complete, and returns its error status.
// It handles errors base on FailMode.
func (c *xClient) Call(ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error {
	if c.isShutdown {
		return ErrXClientShutdown
	}

	if c.auth != "" {
		metadata := ctx.Value(share.ReqMetaDataKey)
		if metadata == nil {
			metadata = map[string]string{}
			ctx = context.WithValue(ctx, share.ReqMetaDataKey, metadata)
		}
		m := metadata.(map[string]string)
		m[share.AuthKey] = c.auth
	}

	var err error
	// todo: xClient在 selectClient中内存初始化为 client
	k, client, err := c.selectClient(ctx, c.servicePath, serviceMethod, args)
	if err != nil {
		if c.failMode == Failfast {
			return err
		}
	}

	var e error
	switch c.failMode {
	case Failtry:
		retries := c.option.Retries
		for retries >= 0 {
			retries--
			logs.Debugf("retries ------------ %+v ", retries)

			if client != nil {
				err = c.wrapCall(ctx, client, serviceMethod, args, reply)
				if err == nil {
					return nil
				}
				if _, ok := err.(ServiceError); ok {
					return err
				}
			}

			if uncoverError(err) {
				c.removeClient(k, client)
			}
			client, e = c.getCachedClient(k)
		}
		if err == nil {
			err = e
		}
		return err
	case Failover:
		retries := c.option.Retries
		for retries >= 0 {
			retries--

			if client != nil {
				err = c.wrapCall(ctx, client, serviceMethod, args, reply)
				if err == nil {
					return nil
				}
				if _, ok := err.(ServiceError); ok {
					return err
				}
			}

			if uncoverError(err) {
				c.removeClient(k, client)
			}
			//select another server
			k, client, e = c.selectClient(ctx, c.servicePath, serviceMethod, args)
		}

		if err == nil {
			err = e
		}
		return err
	case Failbackup:
		ctx, cancelFn := context.WithCancel(ctx)
		defer cancelFn()
		call1 := make(chan *Call, 10)
		call2 := make(chan *Call, 10)

		var reply1, reply2 interface{}

		if reply != nil {
			reply1 = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			reply2 = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
		}

		_, err1 := c.Go(ctx, serviceMethod, args, reply1, call1)

		t := time.NewTimer(c.option.BackupLatency)
		select {
		case <-ctx.Done(): //cancel by context
			err = ctx.Err()
			return err
		case call := <-call1:
			err = call.Error
			if err == nil && reply != nil {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(reply1).Elem())
			}
			return err
		case <-t.C:

		}
		_, err2 := c.Go(ctx, serviceMethod, args, reply2, call2)
		if err2 != nil {
			if uncoverError(err2) {
				c.removeClient(k, client)
			}
			err = err1
			return err
		}

		select {
		case <-ctx.Done(): //cancel by context
			err = ctx.Err()
		case call := <-call1:
			err = call.Error
			if err == nil && reply != nil && reply1 != nil {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(reply1).Elem())
			}
		case call := <-call2:
			err = call.Error
			if err == nil && reply != nil && reply2 != nil {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(reply2).Elem())
			}
		}

		return err
	default: //Failfast
		err = c.wrapCall(ctx, client, serviceMethod, args, reply)
		if err != nil {
			if uncoverError(err) {
				c.removeClient(k, client)
			}
		}

		return err
	}
}


func (c *xClient) CallNotGo(svc string, md string, pairs []*KVPair) string {
	// 调用非go服务
	if len(pairs) == 0 {
		return "Call --- 服务节点数据为空"
	}
	kv := pairs[0]				// fixme： 先选择第一个节点，还需要优化算法
	if strings.Contains(kv.Value, "typ=py") {
		addrSpl := strings.Split(kv.Key, "@")
		return callPySvc(svc, md, addrSpl[1], "{}")
	}

	return ""
}

func callPySvc(svc, md, svcAddr string, params string) string {
	// 调用py服务端, jsonrpc协议
	c := &http.Client{}
	c.Timeout = time.Duration(5 * time.Second)
	req := request.NewRequest(c)
	req.Json = map[string]interface{} {
		"jsonrpc": "2.0",
		"method": svc + "." + md,
		"params": make(map[string]interface{}),				// 空map， 表示为{}
		"id": "1",
	}
	rsp, err := req.Post("http://" + svcAddr + "/api")
	ColorfulRabbit.CheckError(err, "调用服务失败", svcAddr)
	//content, _ := rsp.Content()
	js, _ := rsp.Json()
	rspCt := string(js.Get("result").MustString())
	logs.Debug("reqQw rsp --------------", rsp.StatusCode, rspCt)
	return rspCt
}

func uncoverError(err error) bool {
	if _, ok := err.(ServiceError); ok {
		return false
	}

	if err == context.DeadlineExceeded {
		return false
	}

	if err == context.Canceled {
		return false
	}

	return true
}

func (c *xClient) SendRaw(ctx context.Context, r *protocol.Message) (map[string]string, []byte, error) {
	logs.Debugf("SendRaw c.servers ------------------- %+v", c.servers)
	if len(c.servers) == 0 {
		logs.Debugf("找不到服务节点信息 svc: %+v, md: %+v", r.ServicePath, r.ServiceMethod)
		errMsg := fmt.Sprintf(`{"msg": "找不到服务节点信息 svc: %+v, md: %+v"}`, r.ServicePath, r.ServiceMethod)
		return nil, []byte(errMsg), errors.New(errMsg)
	}

	// fixme： 性能瓶颈测试 start -----------------------------------------------
	/**
	var m map[string]string
	m = make(map[string]string)
	m["X-RPCX-MessageID"] = "2"
	m["X-RPCX-MessageStatusType"] = "Normal"
	m["X-RPCX-Meta"] = ""
	m["X-RPCX-SerializeType"] = "1"
	m["X-RPCX-ServiceMethod"] = "Say"
	m["X-RPCX-ServicePath"] = "Echo"
	m["X-RPCX-Version"] = "0"
	logs.Debug("m ----------------", m)
	payload := []byte("performance debug!")
	logs.Debug("payload ----------------", string(payload))
	return m, payload, nil
	*/
	// fixme： 性能瓶颈测试 end -----------------------------------------------

	if c.isShutdown {
		return nil, nil, ErrXClientShutdown
	}

	if c.auth != "" {
		metadata := ctx.Value(share.ReqMetaDataKey)
		if metadata == nil {
			metadata = map[string]string{}
			ctx = context.WithValue(ctx, share.ReqMetaDataKey, metadata)
		}
		m := metadata.(map[string]string)
		m[share.AuthKey] = c.auth
	}

	var err error
	//logs.Debug("xclient SendRow selectClient -------------")
	// todo: 根据XClient的数据来生成Client，最后的SendRaw逻辑是由Client调用的
	// todo: c.selecrClient 触发 getCachedClient 函数, 这个函数调用了 client/connection.go 的 Connect 函数,
	// todo: 客户端建立给服务端的网络连接
	k, client, err := c.selectClient(ctx, r.ServicePath, r.ServiceMethod, r.Payload)
	//logs.Debug("c.selectClient err ----------", err.(ServiceError), "---", err.Error())
	logs.Debug("DEBUG halokid 2 ------ ")

	if err != nil {
		if c.failMode == Failfast {
			return nil, nil, err
		}

		if _, ok := err.(ServiceError); ok {
			logs.Debug("DEBUG halokid 3 ------ ")
			return nil, nil, err
		}
	}

	var e error
	switch c.failMode {
	case Failtry:
		retries := c.option.Retries
		for retries >= 0 {
			retries--
			if client != nil {
				logs.Debug("client 22222------ %+v", client)
				// fixme: 性能优化点
				m, payload, err := client.SendRaw(ctx, r)
				if err == nil {
					return m, payload, nil
				}
				if _, ok := err.(ServiceError); ok {
					return nil, nil, err
				}
			}

			if uncoverError(err) {
				c.removeClient(k, client)
			}
			client, e = c.getCachedClient(k)
		}

		if err == nil {
			err = e
		}
		return nil, nil, err
	case Failover:			// todo: 这个是gateway默认采用的失败方式
		logs.Debug("DEBUG halokid 4 ------ ")
		retries := c.option.Retries
		logs.Debug("Failover模式 retries --------- %+v ", retries)
		for retries >= 0 {
			retries--
			logs.Info("Failover模式 client --------- %+v ", client)
			if client != nil {
				//logs.Info("client Failover ----- %+v", client)
				m, payload, err := client.SendRaw(ctx, r)
				//logs.Info("Failover模式 err --------- ", err.Error())
				if err == nil {
					logs.Info("SendRaw 得到的 m ---------------- %+v", m)
					//logs.Info("payload ----------------", string(payload))
					return m, payload, nil
				} else {
					logs.Info("[ERROR] ---------------- %+v", err.Error())
				}
				
				if _, ok := err.(ServiceError); ok {
					return nil, nil, err
				}
			}

			if uncoverError(err) {
				c.removeClient(k, client)
			}
			//select another server
			// todo: 这里会重新调用Selector 的 Select方法， 从新选择另外的节点, 默认的Selector 是 roundRobinSelector
			k, client, e = c.selectClient(ctx, r.ServicePath, r.ServiceMethod, r.Payload)
		}

		if err == nil {
			err = e
		}

		logs.Info("DEBUG halokid 5 ------ ")
		return nil, nil, err

	default: //Failfast
		logs.Info("client 44444------ %+v", client)
		m, payload, err := client.SendRaw(ctx, r)

		if err != nil {
			if uncoverError(err) {
				c.removeClient(k, client)
			}
		}

		logs.Info("DEBUG halokid 1 ------ ")
		return m, payload, nil
	}
}

func (c *xClient) wrapCall(ctx context.Context, client RPCClient, serviceMethod string, args interface{}, reply interface{}) error {
	if client == nil {
		return ErrServerUnavailable
	}

	ctx = share.NewContext(ctx)
	// DoPreCall会处理一些opentracking的逻辑, 封装client plugins 的 DoPostCall 方法
	c.Plugins.DoPreCall(ctx, c.servicePath, serviceMethod, args)
	// 调用服务端
	err := client.Call(ctx, c.servicePath, serviceMethod, args, reply)
	// 封装client plugins 的 DoPostCall 方法
	c.Plugins.DoPostCall(ctx, c.servicePath, serviceMethod, args, reply, err)

	return err
}

// Broadcast sends requests to all servers and Success only when all servers return OK.
// FailMode and SelectMode are meanless for this method.
// Please set timeout to avoid hanging.
func (c *xClient) Broadcast(ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error {
	if c.isShutdown {
		return ErrXClientShutdown
	}

	if c.auth != "" {
		metadata := ctx.Value(share.ReqMetaDataKey)
		if metadata == nil {
			metadata = map[string]string{}
			ctx = context.WithValue(ctx, share.ReqMetaDataKey, metadata)
		}
		m := metadata.(map[string]string)
		m[share.AuthKey] = c.auth
	}

	var clients = make(map[string]RPCClient)
	c.mu.Lock()
	for k := range c.servers {
		client, err := c.getCachedClientWithoutLock(k)
		if err != nil {
			continue
		}
		clients[k] = client
	}
	c.mu.Unlock()

	if len(clients) == 0 {
		return ErrXClientNoServer
	}

	var err = &ex.MultiError{}
	l := len(clients)
	done := make(chan bool, l)
	for k, client := range clients {
		k := k
		client := client
		go func() {
			e := c.wrapCall(ctx, client, serviceMethod, args, reply)
			done <- (e == nil)
			if e != nil {
				if uncoverError(err) {
					c.removeClient(k, client)
				}
				err.Append(e)
			}
		}()
	}

	timeout := time.After(time.Minute)
check:
	for {
		select {
		case result := <-done:
			l--
			if l == 0 || !result { // all returns or some one returns an error
				break check
			}
		case <-timeout:
			err.Append(errors.New(("timeout")))
			break check
		}
	}

	if err.Error() == "[]" {
		return nil
	}
	return err
}

// Fork sends requests to all servers and Success once one server returns OK.
// FailMode and SelectMode are meanless for this method.
func (c *xClient) Fork(ctx context.Context, serviceMethod string, args interface{}, reply interface{}) error {
	if c.isShutdown {
		return ErrXClientShutdown
	}

	if c.auth != "" {
		metadata := ctx.Value(share.ReqMetaDataKey)
		if metadata == nil {
			metadata = map[string]string{}
			ctx = context.WithValue(ctx, share.ReqMetaDataKey, metadata)
		}
		m := metadata.(map[string]string)
		m[share.AuthKey] = c.auth
	}

	var clients = make(map[string]RPCClient)
	c.mu.Lock()
	for k := range c.servers {
		client, err := c.getCachedClientWithoutLock(k)
		if err != nil {
			continue
		}
		clients[k] = client
	}
	c.mu.Unlock()

	if len(clients) == 0 {
		return ErrXClientNoServer
	}

	var err = &ex.MultiError{}
	l := len(clients)
	done := make(chan bool, l)
	for k, client := range clients {
		k := k
		client := client
		go func() {
			var clonedReply interface{}
			if reply != nil {
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}

			e := c.wrapCall(ctx, client, serviceMethod, args, clonedReply)
			if e == nil && reply != nil && clonedReply != nil {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
			}
			done <- (e == nil)
			if e != nil {
				if uncoverError(err) {
					c.removeClient(k, client)
				}
				err.Append(e)
			}

		}()
	}

	timeout := time.After(time.Minute)
check:
	for {
		select {
		case result := <-done:
			l--
			if result {
				return nil
			}
			if l == 0 { // all returns or some one returns an error
				break check
			}

		case <-timeout:
			err.Append(errors.New(("timeout")))
			break check
		}
	}

	if err.Error() == "[]" {
		return nil
	}

	return err
}

// SendFile sends a local file to the server.
// fileName is the path of local file.
// rateInBytesPerSecond can limit bandwidth of sending,  0 means does not limit the bandwidth, unit is bytes / second.
func (c *xClient) SendFile(ctx context.Context, fileName string, rateInBytesPerSecond int64) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}

	fi, err := os.Stat(fileName)
	if err != nil {
		return err
	}

	args := serverplugin.FileTransferArgs{
		FileName: fi.Name(),
		FileSize: fi.Size(),
	}

	reply := &serverplugin.FileTransferReply{}
	err = c.Call(ctx, "TransferFile", args, reply)
	if err != nil {
		return err
	}

	conn, err := net.DialTimeout("tcp", reply.Addr, c.option.ConnectTimeout)
	if err != nil {
		return err
	}

	defer conn.Close()

	_, err = conn.Write(reply.Token)
	if err != nil {
		return err
	}

	var tb *ratelimit.Bucket

	if rateInBytesPerSecond > 0 {
		tb = ratelimit.NewBucketWithRate(float64(rateInBytesPerSecond), rateInBytesPerSecond)
	}

	sendBuffer := make([]byte, FileTransferBufferSize)
loop:
	for {
		select {
		case <-ctx.Done():
		default:
			if tb != nil {
				tb.Wait(FileTransferBufferSize)
			}
			n, err := file.Read(sendBuffer)
			if err != nil {
				if err == io.EOF {
					return nil
				} else {
					return err
				}
			}
			if n == 0 {
				break loop
			}
			_, err = conn.Write(sendBuffer)
			if err != nil {
				if err == io.EOF {
					return nil
				} else {
					return err
				}
			}
		}
	}

	return nil
}

func (c *xClient) DownloadFile(ctx context.Context, requestFileName string, saveTo io.Writer) error {
	args := serverplugin.DownloadFileArgs{
		FileName: requestFileName,
	}

	reply := &serverplugin.FileTransferReply{}
	err := c.Call(ctx, "DownloadFile", args, reply)
	if err != nil {
		return err
	}

	conn, err := net.DialTimeout("tcp", reply.Addr, c.option.ConnectTimeout)
	if err != nil {
		return err
	}

	defer conn.Close()

	_, err = conn.Write(reply.Token)
	if err != nil {
		return err
	}

	buf := make([]byte, FileTransferBufferSize)
	r := bufio.NewReader(conn)
loop:
	for {
		select {
		case <-ctx.Done():
		default:
			n, er := r.Read(buf)
			if n > 0 {
				_, ew := saveTo.Write(buf[0:n])
				if ew != nil {
					err = ew
					break loop
				}
			}
			if er != nil {
				if er != io.EOF {
					err = er
				}
				break loop
			}
		}

	}

	return err
}

// Close closes this client and its underlying connnections to services.
func (c *xClient) Close() error {
	c.isShutdown = true

	var errs []error
	c.mu.Lock()
	for k, v := range c.cachedClient {
		e := v.Close()
		if e != nil {
			errs = append(errs, e)
		}

		delete(c.cachedClient, k)

	}
	c.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {

			}
		}()

		c.discovery.RemoveWatcher(c.ch)
		close(c.ch)
	}()

	if len(errs) > 0 {
		return ex.NewMultiError(errs)
	}
	return nil
}


func (c *xClient) IsGo() bool {
	// 检查是否为go服务端	
	return c.isGo
}

func (c *xClient) GetNotGoServers() map[string]string {
	return c.notGoServers
}

func (c *xClient) GetSvcTyp() string {
	return c.svcTyp
}

func (c *xClient) GetReverseProxy() bool {
	return c.isReverseProxy
}



