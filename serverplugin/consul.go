package serverplugin
/**
关于rpcx服务注册的设计说明：
1. 写入服务的folder值在 register方法
2. 写入实际运行节点的逻辑在start方法
这样设计的目的是， 注册与实际节点分开， 因为实际节点信息只有在节点start的时候写入KV才有意义， 但是服务的话，可以先以
folder的方法来注册， 所以这里是注册 与 实际运行 是分开两个概念的， 所以这样设计
 */
import (
	"errors"
	"fmt"
	logs "github.com/halokid/rpcx-plus/log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
	"github.com/docker/libkv/store/consul"
	metrics "github.com/rcrowley/go-metrics"
)

func init() {
	consul.Register()
}

// ConsulRegisterPlugin implements consul registry.
type ConsulRegisterPlugin struct {
	// service address, for example, tcp@127.0.0.1:8972, quic@127.0.0.1:1234
	ServiceAddress string
	// consul addresses
	ConsulServers []string
	// base path for rpcx server, for example com/example/rpcx
	BasePath string
	Metrics  metrics.Registry
	// Registered services
	Services       []string
	metasLock      sync.RWMutex
	metas          map[string]string
	UpdateInterval time.Duration

	Options *store.Config
	kv      store.Store

	dying chan struct{}
	done  chan struct{}				// todo: 空结构体省内存，只是记录状态
}

// Start starts to connect consul cluster
func (p *ConsulRegisterPlugin) Start() error {
	if p.done == nil {
		p.done = make(chan struct{})
	}
	if p.dying == nil {
		p.dying = make(chan struct{})
	}

	if p.kv == nil {
		kv, err := libkv.NewStore(store.CONSUL, p.ConsulServers, p.Options)
		if err != nil {
			logs.Errorf("cannot create consul registry: %v", err)
			return err
		}
		p.kv = kv
	}

	if p.BasePath[0] == '/' {
		p.BasePath = p.BasePath[1:]
	}

	err := p.kv.Put(p.BasePath, []byte("rpcx_path"), &store.WriteOptions{IsDir: true})
	if err != nil {
		logs.Errorf("cannot create consul path %s: %v", p.BasePath, err)
		return err
	}

	if p.UpdateInterval > 0 {
		// todo: 根据服务注册时候设定的时间间隔，定时写入一些服务信息
		ticker := time.NewTicker(p.UpdateInterval)
		go func() {
			// 此gor一直在跑， 所以kv一直不close， 在consul看到的kv的状态是locked
			defer p.kv.Close()

			// refresh service TTL
			for {
				select {
				case <-p.dying:
					close(p.done)
					return						// todo: 退出for select
				case <-ticker.C:
					var data []byte
					if p.Metrics != nil {
						clientMeter := metrics.GetOrRegisterMeter("clientMeter", p.Metrics)
						data = []byte(strconv.FormatInt(clientMeter.Count()/60, 10))
					}

					//set this same metrics for all services at this server
					for _, name := range p.Services {
						// todo: 循环写入KV， 定义注册服务
						nodePath := fmt.Sprintf("%s/%s/%s", p.BasePath, name, p.ServiceAddress)
						kvPaire, err := p.kv.Get(nodePath)
						if err != nil {
							// 如果获取不了key
							logs.Warnf("can't get data of node: %s, will re-create, because of %v", nodePath, err.Error())

							p.metasLock.RLock()
							meta := p.metas[name]
							p.metasLock.RUnlock()
							// 超时时间设置为配置的TTL的 3倍
							err = p.kv.Put(nodePath, []byte(meta), &store.WriteOptions{TTL: p.UpdateInterval * 3})
							if err != nil {
								logs.Errorf("cannot re-create consul path %s: %v", nodePath, err)
							}
						} else {
							v, _ := url.ParseQuery(string(kvPaire.Value))
							v.Set("tps", string(data))
							p.kv.Put(nodePath, []byte(v.Encode()), &store.WriteOptions{TTL: p.UpdateInterval * 3})
						}
					}
				}
			}
		}()
	}

	return nil
}

// Stop unregister all services.
func (p *ConsulRegisterPlugin) Stop() error {
	close(p.dying)
	<-p.done

	if p.kv == nil {
		kv, err := libkv.NewStore(store.CONSUL, p.ConsulServers, p.Options)
		if err != nil {
			logs.Errorf("cannot create consul registry: %v", err)
			return err
		}
		p.kv = kv
	}

	if p.BasePath[0] == '/' {
		p.BasePath = p.BasePath[1:]
	}

	for _, name := range p.Services {
		nodePath := fmt.Sprintf("%s/%s/%s", p.BasePath, name, p.ServiceAddress)
		exist, err := p.kv.Exists(nodePath)
		if err != nil {
			logs.Errorf("cannot delete path %s: %v", nodePath, err)
			continue
		}
		if exist {
			p.kv.Delete(nodePath)
			logs.Debugf("delete path %s", nodePath, err)
		}
	}
	return nil
}

// HandleConnAccept handles connections from clients
func (p *ConsulRegisterPlugin) HandleConnAccept(conn net.Conn) (net.Conn, bool) {
	if p.Metrics != nil {
		clientMeter := metrics.GetOrRegisterMeter("clientMeter", p.Metrics)
		clientMeter.Mark(1)
	}
	return conn, true
}

// Register handles registering event.
// this service is registered at BASE/serviceName/thisIpAddress node
func (p *ConsulRegisterPlugin) Register(name string, rcvr interface{}, metadata string) (err error) {
	/** 实现register plugin的register方法，创建KV的folder名称，设置和名称一样的值，并没有实际写入节点信息 */
	if "" == strings.TrimSpace(name) {
		err = errors.New("Register service `name` can't be empty")
		return
	}

	if p.kv == nil {
		consul.Register()
		// todo: docker.libkv 是一个注册中心中间件封装组件, 定义kv的类型为consul
		kv, err := libkv.NewStore(store.CONSUL, p.ConsulServers, nil)
		if err != nil {
			logs.Errorf("cannot create consul registry: %v", err)
			return err
		}
		p.kv = kv
	}

	if p.BasePath[0] == '/' {
		p.BasePath = p.BasePath[1:]
	}
	err = p.kv.Put(p.BasePath, []byte("rpcx_path"), &store.WriteOptions{IsDir: true})
	if err != nil {
		logs.Errorf("cannot create consul path %s: %v", p.BasePath, err)
		return err
	}

	nodePath := fmt.Sprintf("%s/%s", p.BasePath, name)
	err = p.kv.Put(nodePath, []byte(name), &store.WriteOptions{IsDir: true})
	if err != nil {
		logs.Errorf("cannot create consul path %s: %v", nodePath, err)
		return err
	}

	nodePath = fmt.Sprintf("%s/%s/%s", p.BasePath, name, p.ServiceAddress)
	err = p.kv.Put(nodePath, []byte(metadata), &store.WriteOptions{TTL: p.UpdateInterval * 2})
	if err != nil {
		logs.Errorf("cannot create consul path %s: %v", nodePath, err)
		return err
	}

	p.Services = append(p.Services, name)

	p.metasLock.Lock()
	if p.metas == nil {
		p.metas = make(map[string]string)
	}
	p.metas[name] = metadata
	p.metasLock.Unlock()
	return
}

func (p *ConsulRegisterPlugin) RegisterFunction(serviceName, fname string, fn interface{}, metadata string) error {
	return p.Register(serviceName, fn, metadata)
}

func (p *ConsulRegisterPlugin) Unregister(name string) (err error) {
	if "" == strings.TrimSpace(name) {
		err = errors.New("Unregister service `name` can't be empty")
		return
	}

	if p.kv == nil {
		consul.Register()
		kv, err := libkv.NewStore(store.CONSUL, p.ConsulServers, nil)
		if err != nil {
			logs.Errorf("cannot create consul registry: %v", err)
			return err
		}
		p.kv = kv
	}

	if p.BasePath[0] == '/' {
		p.BasePath = p.BasePath[1:]
	}
	err = p.kv.Put(p.BasePath, []byte("rpcx_path"), &store.WriteOptions{IsDir: true})
	if err != nil {
		logs.Errorf("cannot create consul path %s: %v", p.BasePath, err)
		return err
	}

	nodePath := fmt.Sprintf("%s/%s", p.BasePath, name)

	err = p.kv.Put(nodePath, []byte(name), &store.WriteOptions{IsDir: true})
	if err != nil {
		logs.Errorf("cannot create consul path %s: %v", nodePath, err)
		return err
	}

	nodePath = fmt.Sprintf("%s/%s/%s", p.BasePath, name, p.ServiceAddress)

	err = p.kv.Delete(nodePath)
	if err != nil {
		logs.Errorf("cannot create consul path %s: %v", nodePath, err)
		return err
	}

	var services = make([]string, 0, len(p.Services)-1)
	for _, s := range p.Services {
		if s != name {
			services = append(services, s)
		}
	}
	p.Services = services

	p.metasLock.Lock()
	if p.metas == nil {
		p.metas = make(map[string]string)
	}
	delete(p.metas, name)
	p.metasLock.Unlock()
	return
}
