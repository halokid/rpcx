use consul_rs::Client;

// pub mod config;
#[path = "config.rs"] mod config;

// 服务注册
pub fn regiser(svc_addr: &String) -> bool {
  let mut path_prex = String::from("pomid/");
  let svc = "porust".to_string();

  let port = config::CFG["consul_port"].parse::<u16>().unwrap();
  let c = Client::new(config::CFG["consul_addr"], port);
  // set key
  let key_svc = format!("{}{}", path_prex, svc);
  let ok = c.kv_put(&key_svc, &svc).unwrap();

  println!("写入porust服务信息 {}", ok);
  // set folder
  let key = format!("{}{}/tmp@{}", path_prex, svc, svc_addr);
  let val = String::from("typ=rust");

  let ok = c.kv_put(key, val).unwrap();
  println!("写入porust服务地址 {}", ok);
  ok
  // true
}



