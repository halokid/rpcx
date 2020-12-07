//use serde_json::{Value};
use serde_json;
use tonic::{Request, Response, Status, transport::Server};

use po_rust::{Req, Rsp};
use po_rust::pors_server::{Pors, PorsServer};

#[path = "reg.rs"] mod reg;     // todo: 假如有不同的bin， 出现发现不了mod的情况，可以这样做
#[path = "config.rs"] mod config;

// todo: 装载proto生成的库
pub mod po_rust {
  tonic::include_proto!("porust");
}

#[derive(Debug, Default)]
pub struct DoPors {}

#[tonic::async_trait]
impl Pors for DoPors {
  // todo: rewrite invoke method, 建立invoke方法
  async fn invoke(
    &self,
    request: Request<Req>,
  ) -> Result<Response<Rsp>, Status> {
    println!("<==Svc invoke==>: {:?}", request);

    /* 按照reqdata来返回rsp
    let rsp = po_rust::Rsp {
      // 返回数据
      rspdata: format!("{}", request.into_inner().reqdata).into(),
    };

    Ok(Response::new(rsp))
    */

//    /* 执行 say_hello 返回rsp
    let reqdata = request.into_inner().reqdata;
    let mut handled_rsp = String::new();
    handle_req(reqdata, &mut handled_rsp);

    // todo: 直接执行方法
    let rsp = po_rust::Rsp {
      rspdata: format!("{}", handled_rsp),
    };
    Ok(Response::new(rsp))
  }
}


fn handle_req(reqdata: String, handled_rsp: &mut String) -> serde_json::Result<()> {
  // 统一处理req的数据， 分发call的方法
  // todo: 根据reqdata指定call的方法执行
  /* 请求的数据格式:
  {"call": "say_hi", "data": {"name": "halokid"}}
   */
  let req_js: serde_json::Value = serde_json::from_str(&reqdata)?;
  let call = serde_json::json!(req_js["call"]);
  println!("handle_req call: {}", call);
  let reqdata_data = serde_json::json!(req_js["data"]);

  // todo: 格式为 svc.method
  if call == "porust.say_hi" {
    say_hi(&reqdata_data, handled_rsp);
  }

  Ok(())
}

// 执行say_hello逻辑, 由say_hello返回执行结果
fn say_hello() -> String {
  let sayhello = r#"{"name": "halokid", "root": {"sub": 29}}"#.to_string();
  return sayhello;
}

fn say_hi(reqdata_data: &serde_json::Value, handled_rsp: &mut String) -> serde_json::Result<()> {
  // {"call": "say_hi", "data": {"name": "halokid"}}
  println!("say_hi handle: {}", reqdata_data);
  let name = serde_json::json!(reqdata_data["name"]);
  let namex = name.as_str().unwrap();

  // 把namex的值写进 rspdata
  handled_rsp.push_str(&namex);

  Ok(())
}


#[tokio::main]
pub async fn main() -> Result<(), Box<dyn std::error::Error>> {
  // init
  println!("CFG-- env: {}", config::CFG["env"]);

  // reg
  let addr = local_ipaddress::get().unwrap();
  println!("addr----{}", addr);
  // let s = String::from("127.0.0.1");
  let sp = addr + ":18080";
  let serv_addr = (&sp).parse()?;
  // let addr = "127.0.0.1:18080".parse()?;
  let is_reg = reg::regiser(&sp);
  if is_reg {
    println!("成功注册 porust: {}", sp);
  } else {
    println!("------注册失败-----");
  }

  // run
  let pors = DoPors::default();     // 返回DoPors结构体的默认值

  Server::builder().add_service(PorsServer::new(pors))
    .serve(serv_addr)
    .await?;

  Ok(())
}


#[test]
fn server_test() {
  println!("server test...");
}

#[test]
fn server_run_test() {
  println!("server run test...");
}

