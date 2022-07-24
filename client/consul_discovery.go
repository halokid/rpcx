package client

import (
  "strings"
  "sync"
  "time"

  "github.com/docker/libkv"
  "github.com/docker/libkv/store"
  "github.com/docker/libkv/store/consul"
  logs "github.com/halokid/rpcx-plus/log"
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
    logs.Debugf("cannot create store: %v", err)
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
  //logs.x.Println("basePath --------------", basePath)
  ///**
  // todo: 修复服务名模糊查找错误bug
  //logs.x.Println("svc nodes ------------- ", ps)
  //for i, p := range ps {
  //logs.x.Println("i, p ------------- ", i, p.Key, p.Value)
  //}
  //*/

  if err != nil {
    //logs.Debugf("cannot get services of from registry: %v, err: %v", basePath, err)
    logs.Errorf("[ERROR]------从注册中心找不到服务cannot get services of from registry: %v, err: %v", basePath, err)
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
    logs.Debugf("cannot create store: %v", err)
    panic(err)
  }

  return &ConsulDiscovery{basePath: basePath, kv: kv}
}

// Clone clones this ServiceDiscovery with new servicePath.
func (d *ConsulDiscovery) Clone(servicePath string) ServiceDiscovery {
  logs.Debug("-----@@---- 触发服务节点发现 go d.watch()的逻辑," +
  "现在的 d.watch()逻辑是每一个svc就是有一个对应的gor来watch该svc的节点变化，有多少个svc就有多少个这样的gor" +
  "-- ConsulDiscovery Clone ----@@------ ")
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
  d.chans = append(d.chans, ch)   // todo: if d.chans is not nil, it will add to ch, every time add 10 nodes at most.
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
  logs.Debug("----------- 执行 go d.watch() ------------")
  for {
    var err error
    var c <-chan []*store.KVPair
    var tempDelay time.Duration

    logs.Debugf("实时的consul发现状态 -------- %+v", d)
    retry := d.RetriesAfterWatchFailed
    for d.RetriesAfterWatchFailed < 0 || retry >= 0 {
      logs.Debug("----------- 执行 go d.watch() -> 执行WatchTree ------------")
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
        logs.Warnf("can not watchtree (with retry %d, sleep %v): %s: %v", retry, tempDelay, d.basePath, err)
        time.Sleep(tempDelay)
        continue
      }
      break
    }

    if err != nil {
      logs.Errorf("can't watch %s: %v", d.basePath, err)
      return
    }

    prefix := d.basePath + "/"

    logs.Debug("----------- 执行 go d.watch() -> 完成WatchTree ------------")

   // fixme:
  // todo: 读取节点注册信息的变化， 目前好像只能读取到增加的变化， 不能读取到减少的变化？
  readChanges:
    for {
      select {
      case <-d.stopCh:
        logs.Info("discovery has been closed")
        return

      case ps := <-c:
        logs.Debug("----------- 执行 go d.watch() -> WatchTree -> 监控Tree变化", d.basePath, "------")
        if ps == nil {
          logs.Errorf("ps := <-c，读取到 ps == nil，表示注册中心watch读取到为nil，跳出readChanges")
          break readChanges
        }
        var pairs []*KVPair // latest servers
        logs.Debug("=== WatchTree更新节点数据", d.basePath, "===")
        for i, p := range ps {
          pKeySp := strings.Split(p.Key, "/")
          if len(pKeySp) > 0 && (pKeySp[0]+"/"+pKeySp[1] != d.basePath) {
            continue
          }
          k := strings.TrimPrefix(p.Key, prefix)
          pair := &KVPair{Key: k, Value: string(p.Value)}
          if d.filter != nil && !d.filter(pair) {   // todo: 过滤掉一些已经禁止的节点
            continue
          }
          logs.Debugf("节点index %+v key -->>> %+v, key的val -->>> %+v", i, p.Key, string(p.Value))
          pairs = append(pairs, pair) // 一次loading所有的服务键值对(pairs)
        }
        // todo: ConsulDiscovery 存了两个有关于node service节点信息的数据， 一个是pairs, 一个是chans
        // todo: paire是缓存成本地的nodes service数据等用途， chans是removeWatcher（目前还没发现其他作用）用途来的
        // todo: 获取到的节点数据写入 d.pairs，数据结构是 []*KVPair
        d.pairs = pairs

        d.mu.Lock()
        for _, ch := range d.chans {
          ch := ch
          go func() {
            defer func() {
              if r := recover(); r != any(nil) {

              }
            }()
            select {
            // todo: 获取到的节点数据写入 d.chan，数据结构是 []chan []*KVPair
            case ch <- pairs: // todo: d.chans 为指针， 所以 ch <-pairs 是用 pairs的item去赋值给 d.chans的 item
            case <-time.After(time.Minute): // 每一次用pairs填充 d.chans 容量的变化， 最大填充的容量为len(d.chans)
              logs.Warn("chan is full and new change has been dropped")
            }
          }()
        }
        d.mu.Unlock()

      //todo: 假如两个case的情况都匹配不了，不断的for循环，会造成cpu暴涨300%，所以加上default延迟响应
      // todo: 因为本地保存有服务的缓存信息， 所以就算暂时连接不了consul，更新不了缓存信息，服务也不会有影响
      //default:
        //time.Sleep(3 * time.Second)
        //return		// todo: 不要加返回，不然就会跳出 watch(), 从而监察不了服务列表的变化了, 默认情况下，更改了服务节点信息， 最慢3秒更新
      }
    }

    logs.Warn("chan is closed and will rewatch")
  }   // END FOR
}

func (d *ConsulDiscovery) Close() {
  close(d.stopCh)
}
