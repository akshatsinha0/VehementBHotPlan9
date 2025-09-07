use std::ffi::{CStr, CString};
use std::os::raw::{c_char, c_longlong};
use std::sync::Arc;
use std::sync::atomic::{AtomicI64, AtomicU64, Ordering};
use std::collections::HashMap;
use std::hash::BuildHasher;
use once_cell::sync::Lazy;
use ahash::RandomState;
use std::sync::RwLock;

const SHARDS: usize = 64;

struct Cell {
    v: AtomicI64,
    ts: AtomicU64,
}

impl Cell {
    fn new(init: i64, t: u64) -> Self {
        Self {  v: AtomicI64::new(init), ts: AtomicU64::new(t) }
    }
    fn load(&self) -> (i64, u64) {
        (self.v.load(Ordering::Relaxed), self.ts.load(Ordering::Relaxed))
    }
    fn store(&self, x: i64, t:u64) {
        self.v.store(x, Ordering::Relaxed);
        self.ts.store(t, Ordering::Relaxed);
    }
    fn add(&self, x: i64, t: u64) {
        self.v.fetch_add(x, Ordering::Relaxed);
        self.ts.store(t, Ordering::Relaxed);
    }
}

struct Sharded {
    hasher: RandomState,
    shards: Vec<RwLock<HashMap<String, Arc<Cell>, RandomState>>>,
}

impl Sharded {
    fn new() -> Self {
        let h = RandomState::new();
        let mut v = Vec::with_capacity(SHARDS);
        for _ in 0..SHARDS {
            v.push(RwLock::new(HashMap::with_hasher(RandomState::new())));
        }
        Self { hasher: h, shards: v }
    }
    fn idx(&self, k: &str) -> usize {
        (self.hasher.hash_one(k) as usize) & (SHARDS - 1)
    }
    fn get_or_insert_counter(&self, k: &str, now: u64) -> Arc<Cell> {
        let i = self.idx(k);
        {
            let g = self.shards[i].read().unwrap();
            if let Some(x) = g.get(k) {
                return Arc::clone(x);
            }
        }
        let mut g = self.shards[i].write().unwrap();
        if let Some(x) = g.get(k) {
            return Arc::clone(x);
        }
        let c = Arc::new(Cell::new(0, now));
        g.insert(k.to_string(), Arc::clone(&c));
        c
    }
    fn get_or_insert_gauge(&self, k: &str, now: u64) -> Arc<Cell> {
        let i = self.idx(k);
        {
            let g = self.shards[i].read().unwrap();
            if let Some(x) = g.get(k) {
                return Arc::clone(x);
            }
        }
        let mut g = self.shards[i].write().unwrap();
        if let Some(x) = g.get(k) {
            return Arc::clone(x);
        }
        let c = Arc::new(Cell::new(0, now));
        g.insert(k.to_string(), Arc::clone(&c));
        c
    }
    fn remove(&self, k: &str) {
        let i = self.idx(k);
        let mut g = self.shards[i].write().unwrap();
        g.remove(k);
    }
    fn snapshot(&self) -> Vec<(String, i64, u64)> {
        let mut out = Vec::new();
        for s in &self.shards {
            let g = s.read().unwrap();
            for (k, c) in g.iter() {
                let (v, t) = c.load();
                out.push((k.clone(), v, t));
            }
        }
        out
    }
}
static COUNTERS: Lazy<Sharded> = Lazy::new(|| Sharded::new());
static GAUGES:   Lazy<Sharded> = Lazy::new(|| Sharded::new());

fn now_mono() -> u64 {
    use std::time::Instant;
    static BOOT: Lazy<Instant> = Lazy::new(Instant::now);
    BOOT.elapsed().as_nanos() as u64
}
fn cstr_to_string(p: *const c_char) -> Option<String> {
    if p.is_null() { return None; }
    let s = unsafe { CStr::from_ptr(p) };
    let raw = s.to_bytes();
    let mut out = String::with_capacity(raw.len());
    for &b in raw {
        if b == 0 { break; }
        out.push(b as char);
    }
    Some(out)
}

fn sanitize_key(k: &str)  -> String {
    let mut o = String::with_capacity(k.len());
    for ch in k.chars() {
        if ch == '\n' || ch == '\r' || ch == '\0' { continue; }
        o.push(ch);
    }
    o
}

fn encode_lines(mut  rows: Vec<(String, i64, u64)>) -> CString {
    rows.sort_unstable_by(|a,b| a.0.cmp(&b.0));
    let mut cap = 0usize;
    for (k, v, _) in &rows {
        cap += k.len() +  1 + num_len(*v) + 1;
    }
    let mut s = String::with_capacity(cap);
    for (k, v, t) in rows {
        s.push_str(&k);
        s.push(' ');
        itoa_into(v, &mut s);
        s.push('\n');
        let _ = t;
    }
    CString::new(s).unwrap_or_else(|_| CString::new("").unwrap())
}

fn num_len(mut x: i64) -> usize {
    if x == 0 { return 1; }
    let neg = x < 0;
    if neg { x = -x; }
    let mut n = 0;
    while x > 0 { n += 1; x /= 10; }
    if neg { n + 1 } else { n }
}

fn itoa_into(mut x: i64, s: &mut String) {
    if x == 0 { s.push('0'); return; }
    let neg = x < 0;
    if neg { x =-x; }
    let mut buf = [0u8;  20];
    let mut i = 0;
    while x > 0 {
        let d = (x % 10) as u8;
        buf[i] = b'0' + d;
        i += 1;
        x /= 10;
    }
    if neg { s.push('-'); }
    while i > 0 {
        i -= 1;
        s.push(buf[i] as char);
    }
}

#[no_mangle]
pub extern "C" fn metrics_inc_counter(k: *const c_char, v: c_longlong) {
    if let Some(mut key) = cstr_to_string(k) {
        key = sanitize_key(&key);
        if key.is_empty() { return; }
        let now = now_mono();
        let cell = COUNTERS.get_or_insert_counter(&key, now);
        cell.add(v as i64, now);
    }
}

#[no_mangle]
pub extern "C" fn metrics_set_gauge(k: *const c_char, v: c_longlong) {
    if let Some(mut key) = cstr_to_string(k) {
        key = sanitize_key(&key);
        if key.is_empty() { return; }
        let now = now_mono();
        let cell = GAUGES.get_or_insert_gauge(&key, now);
        cell.store(v as i64, now);
    }
}

#[no_mangle]
pub extern "C" fn metrics_reset_counter(k: *const c_char) {
    if let Some(mut key)=cstr_to_string(k) {
        key = sanitize_key(&key);
        if key.is_empty() { return; }
        COUNTERS.remove(&key);
    }
}

#[no_mangle]
pub extern "C" fn metrics_reset_gauge(k: *const c_char) {
    if let Some(mut key) = cstr_to_string(k) {
        key = sanitize_key(&key);
        if key.is_empty() { return; }
        GAUGES.remove(&key);
    }
}

#[no_mangle]
pub extern "C" fn metrics_dump_counters() -> *mut c_char {
    let snap = COUNTERS.snapshot();
    let c = encode_lines(snap);
    c.into_raw()
}

#[no_mangle]
pub extern "C" fn metrics_dump_gauges() -> *mut c_char {
    let snap = GAUGES.snapshot();
    let c = encode_lines(snap);
    c.into_raw()
}
#[no_mangle]
pub extern "C" fn metrics_free(p: *mut c_char) {
    if p.is_null() { return; }
    unsafe { let _ = CString::from_raw(p); }
}
