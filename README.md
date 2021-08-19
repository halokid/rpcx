
## rpcx-plus
The Origin Project site: [http://rpcx.io](http://rpcx.io/)

this project is my clone secondary development version, the  origin project readme is <a href="README_origin.md">link</a>

**it not only support go, also support python, rust, java(coding now).**

different language communication use rpcx-plus-gateway for proxy, check this <a href="https://github.com/halokid/rpcx-plus-gateway">link</a>.

a service expose for a http api use gateway  
 

## Features
- [x] support python jsonrpc service invoke
- [x] support python client invoke go service 
- [ ] support python client invoke rust service 
- [x] fix the online status of python server, display in web UI 
- [x] add jaeger support plugin, ./serverplugin/jaeger.go
- [x] support rust grpc service invoke
- [ ] support rust client invoke go service
- [ ] support rust client invoke python service
- [ ] support java grpc service invoke
- [ ] support java client invoke go & rust & python service


## Install
for compatibility, original rpcx code, use the original type to install

```shell script
go get -v -tags "quic kcp ping utp" github.com/halokid/rpcx-plus/... 

```
if you GO111MODULE=on, you can do the same thing in your $GOPATH/pkg folder

## Update
```shell script
cd  $GOPATH/github.com/smallnest/rpcx && git pull
```

Enjoy!
