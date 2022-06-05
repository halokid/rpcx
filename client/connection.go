package client

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	logs "github.com/halokid/rpcx-plus/log"
	"github.com/halokid/rpcx-plus/share"
)

type ConnFactoryFn func(c *Client, network, address string) (net.Conn, error)

var ConnFactories = map[string]ConnFactoryFn{
	"http": newDirectHTTPConn,
	"kcp":  newDirectKCPConn,
	"quic": newDirectQuicConn,
	"unix": newDirectConn,
}

// Connect connects the server via specified network.
func (c *Client) Connect(network, address string) error {
	var conn net.Conn
	var err error

	switch network {
	case "http":
		conn, err = newDirectHTTPConn(c, network, address)
	case "kcp":
		conn, err = newDirectKCPConn(c, network, address)
	case "quic":
		conn, err = newDirectQuicConn(c, network, address)
	case "unix":
		logs.Debug("unix case ---------------")
		conn, err = newDirectConn(c, network, address)
	default:
		//logx.Println("default case ---------------")
		fn := ConnFactories[network]
		if fn != nil {
			conn, err = fn(c, network, address)
		} else {
			conn, err = newDirectConn(c, network, address)
		}
	}

	if err == nil && conn != nil {
		if c.option.ReadTimeout != 0 {
			conn.SetReadDeadline(time.Now().Add(c.option.ReadTimeout))
		}
		if c.option.WriteTimeout != 0 {
			conn.SetWriteDeadline(time.Now().Add(c.option.WriteTimeout))
		}

		if c.Plugins != nil {
			conn, err = c.Plugins.DoConnCreated(conn)
			if err != nil {
				return err
			}
		}

		c.Conn = conn
		// todo: 这里是读取服务端返回数据的关键， c.r = bufio.NewReaderSize(conn, ReaderBuffsize)这样定义是表示从 conn 的返回之后获取数据, 这样的话在 server.go 的 serverConn 函数里面的 conn.Write(data) 就是直接把返回的数据写入这个 conn， 所以成功写入返回数据之后， c.r 的数据就会改变了，然后	input函数一直在一个gor运行， 就会不断读取这个 c.r， 读取到之后， 就会赋值给 call.Reply， 返回给客户端了
		// todo: gateway的直接的 SendRaw 的逻辑之前觉得在client.Conn.Write()之后就没有获取返回服务端的返回，其实关键就是在这里，client.Conn.Write()发送给服务端
		// todo: 之后， 这个 c.r 立即就获取了服务端的返回, 然后 go.c.input() 处理 c.r， 写入call.reply, 执行call.done()表示call已完成
		c.r = bufio.NewReaderSize(conn, ReaderBuffsize)
		//logs.Debugf("len: client.r 1 ---------------------  %+v", c.r)
		//c.w = bufio.NewWriterSize(conn, WriterBuffsize)

		// start reading and writing since connected
		go c.input()

		if c.option.Heartbeat && c.option.HeartbeatInterval > 0 {
			go c.heartbeat()
		}

	}

	return err
}

func newDirectConn(c *Client, network, address string) (net.Conn, error) {
	//logx.Println("network ----------------", network)
	//logx.Println("address ----------------", address)
	var conn net.Conn
	var tlsConn *tls.Conn
	var err error

	if c != nil && c.option.TLSConfig != nil {
		dialer := &net.Dialer{
			Timeout: c.option.ConnectTimeout,
		}
		logs.Debug("tls.DialWithDialer 建立client 和 server的连接")
		tlsConn, err = tls.DialWithDialer(dialer, network, address, c.option.TLSConfig)
		//or conn:= tls.Client(netConn, &config)
		conn = net.Conn(tlsConn)
	} else {
		logs.Debug("匹配到 c *Client == nil 或者 c.option.TLSConfig == nil 的情况  ")
		logs.Debugf("c *Client: %+v,  c.option.TLSConfig: %+v", c, c.option.TLSConfig)
		logs.Debugf("net.DialTimeout 建立client和server的连接, timeout: %+v", c.option.ConnectTimeout)
		conn, err = net.DialTimeout(network, address, c.option.ConnectTimeout)
	}

	if err != nil {
		//logs.Warnf("failed to dial server: %v", err)
		logs.Warnf("与服务端通信失败: %v, 服务端为: %+v", err, address)
		return nil, err
	}

	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(3 * time.Minute)
	}

	return conn, nil
}

var connected = "200 Connected to rpcx"

func newDirectHTTPConn(c *Client, network, address string) (net.Conn, error) {
	path := c.option.RPCPath
	if path == "" {
		path = share.DefaultRPCPath
	}

	var conn net.Conn
	var tlsConn *tls.Conn
	var err error

	if c != nil && c.option.TLSConfig != nil {
		dialer := &net.Dialer{
			Timeout: c.option.ConnectTimeout,
		}
		tlsConn, err = tls.DialWithDialer(dialer, "tcp", address, c.option.TLSConfig)
		//or conn:= tls.Client(netConn, &config)

		conn = net.Conn(tlsConn)
	} else {
		conn, err = net.DialTimeout("tcp", address, c.option.ConnectTimeout)
	}
	if err != nil {
		logs.Errorf("failed to dial server: %v", err)
		return nil, err
	}

	io.WriteString(conn, "CONNECT "+path+" HTTP/1.0\n\n")

	// Require successful HTTP response
	// before switching to RPC protocol.
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == connected {
		return conn, nil
	}
	if err == nil {
		logs.Errorf("unexpected HTTP response: %v", err)
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	conn.Close()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  network + " " + address,
		Addr: nil,
		Err:  err,
	}
}
