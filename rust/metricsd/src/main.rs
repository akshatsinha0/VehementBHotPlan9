use std::sync::Arc;
use dashmap::DashMap;
use tokio::net::{TcpListener, TcpStream};
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};

#[derive(Clone)]
struct State {
    counters: Arc<DashMap<String,i64>>,
    gauges: Arc<DashMap<String,i64>>,
}

#[tokio::main]
async fn main() {
    let st = State{ counters: Arc::new(DashMap::new()), gauges: Arc::new(DashMap::new()) };
    let ln = TcpListener::bind("127.0.0.1:9630").await.unwrap();
    loop {
        let (c,_) = ln.accept().await.unwrap();
        let stc = st.clone();
        tokio::spawn(async move { handle(c, stc).await; });
    }
}

async fn handle(c: TcpStream, st: State) {
    let (r, mut w) = c.into_split();
    let mut br = BufReader::new(r);
    let mut line = String::new();
    loop {
        line.clear();
        if br.read_line(&mut line).await.unwrap() == 0 { break }
        let l = line.trim();
        if l.is_empty() { continue }
        let mut it = l.split_whitespace();
        let cmd = it.next().unwrap_or("");
        match cmd {
            "inc" => {
                let k = it.next().unwrap_or("");
                let v = it.next().unwrap_or("0").parse::<i64>().unwrap_or(0);
                st.counters.entry(k.to_string()).and_modify(|e| *e += v).or_insert(v);
                w.write_all(b".\n").await.unwrap();
            }
            "set" => {
                let k = it.next().unwrap_or("");
                let v = it.next().unwrap_or("0").parse::<i64>().unwrap_or(0);
                st.gauges.insert(k.to_string(), v);
                w.write_all(b".\n").await.unwrap();
            }
            "rc" => {
                let k = it.next().unwrap_or("");
                st.counters.remove(k);
                w.write_all(b".\n").await.unwrap();
            }
            "rg" => {
                let k = it.next().unwrap_or("");
                st.gauges.remove(k);
                w.write_all(b".\n").await.unwrap();
            }
            "dc" => {
                let mut v: Vec<_> = st.counters.iter().map(|e| (e.key().clone(), *e.value())).collect();
                v.sort_by(|a,b| a.0.cmp(&b.0));
                for (k,val) in v {
                    w.write_all(format!("{} {}\n", k, val).as_bytes()).await.unwrap();
                }
                w.write_all(b".\n").await.unwrap();
            }
            "dg" => {
                let mut v: Vec<_> = st.gauges.iter().map(|e| (e.key().clone(), *e.value())).collect();
                v.sort_by(|a,b| a.0.cmp(&b.0));
                for (k,val) in v {
                    w.write_all(format!("{} {}\n", k, val).as_bytes()).await.unwrap();
                }
                w.write_all(b".\n").await.unwrap();
            }
            _ => { w.write_all(b".\n").await.unwrap(); }
            /*
                nanoPiyal AbBa ButterBarkwithd
            */
        }
    }
}