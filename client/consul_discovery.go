package client

import (
  "strings"
  "sync"
  "time"

  "github.com/docker/libkv"
  "github.com/docker/libkv/store"
  "github.com/docker/libkv/store/consul"
  "github.com/halokid/rpcx-plus/log"
  log2 "github.com/halokid/rpcx-plus/log"
)

func init() {
  consul.Register()
}

// ConsulDiscovery is a consul service discovery.
// It always returns the registered servers in consul.
type ConsulDiscovery struct {
  basePath string
  kv       store.Store
  pairs    []*KVPair
  chans    []chan []*KVPair
  mu       sync.Mutex
  // -1 means it always retry to watch until zookeeper is ok, 0 means no retry.
  RetriesAfterWatchFailed int

  filter ServiceDiscoveryFilter

  stopCh chan struct{}
}

// NewConsulDiscovery returns a new ConsulDiscovery.
func NewConsulDiscovery(basePath, servicePath string, consulAddr []string, options *store.Config) ServiceDiscovery {
  kv, err := libkv.NewStore(store.CONSUL, consulAddr, options)
  if err != nil {
    log.Infof("cannot create store: %v", err)
    panic(err)
  }

  return NewConsulDiscoveryStore(basePath+"/"+servicePath, kv)
}

// NewConsulDiscoveryStore returns a new ConsulDiscovery with specified store.
func NewConsulDiscoveryStore(basePath string, kv store.Store) ServiceDiscovery {
  if basePath[0] == '/' {
    basePath = basePath[1:]
  }

  if len(basePath) > 1 && strings.HasSuffix(basePath, "/") {
    basePath = basePath[:len(basePath)-1]
  }

  d := &ConsulDiscovery{basePath: basePath, kv: kv}
  d.stopCh = make(chan struct{})

  ps, err := kv.List(basePath)
  //logx.Println("basePath --------------", basePath)
  ///**
  // todo: 修复服务名模糊查找错误bug
  //logx.Println("svc nodes ------------- ", ps)
  //for i, p := range ps {
  //logx.Println("i, p ------------- ", i, p.Key, p.Value)
  //}
  //*/

  if err != nil {
    //log.Infof("cannot get services of from registry: %v, err: %v", basePath, err)
    log.Infof("[ERROR]------从注册中心找不到服务cannot get services of from registry: %v, err: %v", basePath, err)
    //panic(err)
    // todo: 找不到服务不需要进程崩溃
    return d
  }

  var pairs = make([]*KVPair, 0, len(ps))
  prefix := d.basePath + "/"
  for _, p := range ps {
    pKeySp := strings.Split(p.Key, "/")
    if len(pKeySp) > 0 && (pKeySp[0]+"/"+pKeySp[1] != basePath) {
      continue
    }
    k := strings.TrimPrefix(p.Key, prefix)
    pair := &KVPair{Key: k, Value: string(p.Value)}
    if d.filter != nil && !d.filter(pair) {
      continue
    }
    pairs = append(pairs, pair)
  }
  d.pairs = pairs
  d.RetriesAfterWatchFailed = -1
  // todo: 这里是监控服务节点的变化的逻辑， 原来修复了相似服务名的bug之后，再次访问会再出现是因为这里监控了节点的变化
  // todo: 又把去掉的非法服务重新写进了 c.servers，才导致的
  go d.watch()
  return d
}

// NewConsulDiscoveryTemplate returns a new ConsulDiscovery template.
func NewConsulDiscoveryTemplate(basePath string, consulAddr []string, options *store.Config) ServiceDiscovery {
  if basePath[0] == '/' {
    basePath = basePath[1:]
  }

  if len(basePath) > 1 && strings.HasSuffix(basePath, "/") {
    basePath = basePath[:len(basePath)-1]
  }

  kv, err := libkv.NewStore(store.CONSUL, consulAddr, options)
  if err != nil {
    log.Infof("cannot create store: %v", err)
    panic(err)
  }

  return &ConsulDiscovery{basePath: basePath, kv: kv}
}

// Clone clones this ServiceDiscovery with new servicePath.
func (d *ConsulDiscovery) Clone(servicePath string) ServiceDiscovery {
  log2.ADebug.Print("-----@@---- ConsulDiscovery Clone ----@@------ ")
  return NewConsulDiscoveryStore(d.basePath+"/"+servicePath, d.kv)
}

// SetFilter sets the filer.
func (d *ConsulDiscovery) SetFilter(filter ServiceDiscoveryFilter) {
  d.filter = filter
}

// GetServices returns the servers
func (d *ConsulDiscovery) GetServices() []*KVPair {
  return d.pairs
}

// WatchService returns a nil chan.
func (d *ConsulDiscovery) WatchService() chan []*KVPair {
  d.mu.Lock()
  defer d.mu.Unlock()

  ch := make(chan []*KVPair, 10)
  d.chans = append(d.chans, ch)
  return ch
}

func (d *ConsulDiscovery) RemoveWatcher(ch chan []*KVPair) {
  d.mu.Lock()
  defer d.mu.Unlock()

  var chans []chan []*KVPair
  for _, c := range d.chans {
    if c == ch {
      continue
    }

    chans = append(chans, c)
  }

  d.chans = chans
}

func (d *ConsulDiscovery) watch() {
  /** todo: 定时更新服务的节点信息， 初次读取服务之后会写入缓存，后续就靠这个来定时更新服务节点 */
  log2.ADebug.Print("----------- 执行 go d.watch() ------------")
  for {
    var err error
    var c <-chan []*store.KVPair
    var tempDelay time.Duration

    retry := d.RetriesAfterWatchFailed
    for d.RetriesAfterWatchFailed < 0 || retry >= 0 {
      log2.ADebug.Print("----------- 执行 go d.watch() -> 执行WatchTree ------------")
      c, err = d.kv.WatchTree(d.basePath, nil)    // todo: 启用异步gor写入c
      if err != nil {
        if d.RetriesAfterWatchFailed > 0 {
          retry--
        }
        if tempDelay == 0 {
          tempDelay = 1 * time.Second
        } else {
          tempDelay *= 2
        }
        if max := 30 * time.Second; tempDelay > max {
          tempDelay = max
        }
        log.Warnf("can not watchtree (with retry %d, sleep %v): %s: %v", retry, tempDelay, d.basePath, err)
        time.Sleep(tempDelay)
        continue
      }
      break
    }

    if err != nil {
      log.Errorf("can't watch %s: %v", d.basePath, err)
      return
    }

    prefix := d.basePath + "/"

    log2.ADebug.Print("----------- 执行 go d.watch() -> 完成WatchTree ------------")

  readChanges:
    for {
      select {
      case <-d.stopCh:
        log.Info("discovery has been closed")
        return

      case ps := <-c:
        log2.ADebug.Print("----------- 执行 go d.watch() -> WatchTree -> 监控Tree变化 ------")
        if ps == nil {
          log.Errorf("ps := <-c，读取到 ps == nil，表示注册中心watch读取到为nil，跳出readChanges")
          break readChanges
        }
        var pairs []*KVPair // latest servers
        for _, p := range ps {
          log2.ADebug.Print("watch nodes -------------- %+v, %+v, %+v", p.Key, ": ", string(p.Value))
          pKeySp := strings.Split(p.Key, "/")
          if len(pKeySp) > 0 && (pKeySp[0]+"/"+pKeySp[1] != d.basePath) {
            continue
          }
          k := strings.TrimPrefix(p.Key, prefix)
          pair := &KVPair{Key: k, Value: string(p.Value)}
          if d.filter != nil && !d.filter(pair) {
            continue
          }
          pairs = append(pairs, pair) // 一次loading所有的服务键值对(pairs)
        }
        d.pairs = pairs

        d.mu.Lock()
        for _, ch := range d.chans {
          ch := ch
          go func() {
            defer func() {
              if r := recover(); r != nil {

              }
            }()
            select {
            case ch <- pairs: // todo: d.chans 为指针， 所以 ch <-pairs 是用 pairs的item去赋值给 d.chans的 item
            case <-time.After(time.Minute): // 每一次用pairs填充 d.chans 容量的变化， 最大填充的容量为len(d.chans)
              log.Warn("chan is full and new change has been dropped")
            }
          }()
        }
        d.mu.Unlock()

      //todo: 假如两个case的情况都匹配不了，不断的for循环，会造成cpu暴涨300%，所以加上default延迟响应
      // todo: 因为本地保存有服务的缓存信息， 所以就算暂时连接不了consul，更新不了缓存信息，服务也不会有影响
      default:
        time.Sleep(3 * time.Second)
        //return		// todo: 不要加返回，不然就会跳出 watch(), 从而监察不了服务列表的变化了, 默认情况下，更改了服务节点信息， 最慢3秒更新
      }
    }

    log.Warn("chan is closed and will rewatch")
  }   // END FOR
}

func (d *ConsulDiscovery) Close() {
  close(d.stopCh)
}
