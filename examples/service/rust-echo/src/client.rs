//#![feature(core_intrinsics)]


// todo: 装载proto生成的库
pub mod po_rust {
  tonic::include_proto!("porust");
}

use po_rust::pors_client::PorsClient;
use po_rust::Req;

#[tokio::main]
pub async fn invoke(addr: String, req_data: String, rsp: &mut String) -> Result<(), Box<dyn std::error::Error>> {
  // 调用server的invoke方法
  let mut client = PorsClient::connect(addr).await?;

  let request = tonic::Request::new(Req {
//    reqdata:  "halokid".into(),
    reqdata: req_data.into(),
  });

  let response = client.invoke(request).await?;

//  println!("返回: {:?}", response.into_inner().rspdata);
//  print_type_of(&response.into_inner().rspdata);
//  rsp = response.into_inner().rspdata;
  // todo: 赋值给rsp
  rsp.push_str(&response.into_inner().rspdata.to_string());
//  rsp.push_str("xxxxx");

  Ok(())
}


//fn print_type_of<T>(_: &T) {
//  println!("{}", unsafe { std::intrinsics::type_name::<T>() });
//}

#[tokio::main]
async fn  main() -> Result<(), Box<dyn std::error::Error>> {
  let mut client = PorsClient::connect("http://127.0.0.1:18080").await?;

  let request = tonic::Request::new(Req {
    reqdata:  "halokid".into(),
  });

  let response = client.invoke(request).await?;

  println!("返回: {:?}", response);

  Ok(())
}
