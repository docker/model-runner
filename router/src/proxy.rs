//! Backend dispatch: network reverse-proxy or direct in-process call into
//! Go's http.Handler via a streaming C callback.
//!
//! The GoHandler path:
//! 1. Rust creates a tokio::sync::mpsc channel.
//! 2. The raw sender is cast to *mut c_void and written into DmrResponse.stream_ctx.
//! 3. Go's ServeHTTP runs on a spawn_blocking thread; every Write/Flush call
//!    invokes dmr_write_chunk() which sends the chunk into the channel.
//! 4. When ServeHTTP returns, Go calls dmr_close_stream(), which drops the sender.
//! 5. Rust polls the mpsc receiver as a streaming Body, forwarding chunks to the
//!    axum client as they arrive — full streaming with no buffering.

use std::os::raw::{c_void, c_int};
use std::path::PathBuf;
use std::sync::Arc;

use axum::body::Body;
use axum::extract::Request;
use axum::http::{HeaderName, HeaderValue, StatusCode, Uri};
use axum::response::{IntoResponse, Response};
use bytes::Bytes;
use http_body_util::{BodyExt, Full};
use hyper::body::Incoming;
use hyper_util::client::legacy::connect::HttpConnector;
use hyper_util::client::legacy::Client;
use hyper_util::rt::TokioExecutor;
use tokio::net::UnixStream;
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tokio_stream::StreamExt as TokioStreamExt;
use tracing::warn;

// ── FFI types (must match dmr_router.h exactly) ──────────────────────────────

#[repr(C)]
pub struct DmrBytes {
    pub ptr: *mut u8,
    pub len: usize,
}

#[repr(C)]
pub struct DmrRequest<'a> {
    pub method:       *const std::os::raw::c_char,
    pub path:         *const std::os::raw::c_char,
    pub header_block: DmrBytes,
    pub body:         DmrBytes,
    _lifetime: std::marker::PhantomData<&'a ()>,
}

/// Response struct passed to Go.
/// Go sets `status` and `header_block`, then calls `dmr_write_chunk` /
/// `dmr_close_stream` using `stream_ctx` for the body.
#[repr(C)]
pub struct DmrResponse {
    pub status:       u16,
    pub header_block: DmrBytes,
    pub stream_ctx:   *mut c_void,
}

pub type DmrHandlerFn =
    unsafe extern "C" fn(ctx: *mut c_void, req: *const DmrRequest<'_>, resp: *mut DmrResponse);

// ── Streaming C exports ───────────────────────────────────────────────────────

/// The item type carried through the mpsc channel.
/// `None` = end of stream (dmr_close_stream was called).
type ChunkSender = mpsc::Sender<Option<Bytes>>;

/// Send one body chunk from Go into the Rust mpsc channel.
///
/// # Safety
/// `stream_ctx` must be a raw pointer obtained from `Box::into_raw::<ChunkSender>`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dmr_write_chunk(
    stream_ctx: *mut c_void,
    data:        *const u8,
    len:         usize,
) -> c_int {
    if stream_ctx.is_null() {
        return -1;
    }
    let tx: &ChunkSender = unsafe { &*(stream_ctx as *const ChunkSender) };
    if len == 0 {
        // Flush hint — no data, just a signal; nothing to send.
        return 0;
    }
    let chunk = Bytes::copy_from_slice(unsafe { std::slice::from_raw_parts(data, len) });
    // try_send so we never block (the channel has capacity 64).
    match tx.try_send(Some(chunk)) {
        Ok(()) => 0,
        Err(_) => -1, // client disconnected or buffer full
    }
}

/// Signal end-of-body and free the sender.
///
/// # Safety
/// `stream_ctx` must be a pointer previously obtained from
/// `Box::into_raw::<ChunkSender>` and must not have been freed already.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dmr_close_stream(stream_ctx: *mut c_void) {
    if stream_ctx.is_null() {
        return;
    }
    // Dropping the Box drops the Sender, closing the channel.
    let _ = unsafe { Box::from_raw(stream_ctx as *mut ChunkSender) };
}

/// Free a DmrBytes buffer allocated by the Rust side.
unsafe fn dmr_free_bytes(b: DmrBytes) {
    if !b.ptr.is_null() && b.len > 0 {
        drop(unsafe { Vec::from_raw_parts(b.ptr, b.len, b.len) });
    }
}

// ── GoHandlerInner ────────────────────────────────────────────────────────────

struct GoHandlerInner {
    handler_fn:  DmrHandlerFn,
    handler_ctx: *mut c_void,
}

// SAFETY: handler_ctx is a Go cgo.Handle (uintptr_t cast to pointer).
unsafe impl Send for GoHandlerInner {}
unsafe impl Sync for GoHandlerInner {}

// ── BackendClient ─────────────────────────────────────────────────────────────

#[derive(Clone)]
enum BackendAddr {
    GoHandler(Arc<GoHandlerInner>),
    Unix(PathBuf),
    Tcp(u16),
}

#[derive(Clone)]
pub struct BackendClient {
    addr:       BackendAddr,
    tcp_client: Option<Arc<Client<HttpConnector, Body>>>,
}

impl BackendClient {
    /// # Safety
    /// `handler_fn` must remain valid for the lifetime of this `BackendClient`.
    pub unsafe fn new_go(handler_fn: DmrHandlerFn, handler_ctx: *mut c_void) -> Self {
        Self {
            addr: BackendAddr::GoHandler(Arc::new(GoHandlerInner { handler_fn, handler_ctx })),
            tcp_client: None,
        }
    }

    pub fn new_unix(path: PathBuf) -> Self {
        Self { addr: BackendAddr::Unix(path), tcp_client: None }
    }

    pub fn new_tcp(port: u16) -> Self {
        let connector = HttpConnector::new();
        let client    = Client::builder(TokioExecutor::new()).build(connector);
        Self { addr: BackendAddr::Tcp(port), tcp_client: Some(Arc::new(client)) }
    }

    pub async fn proxy(&self, req: Request, target_path: &str) -> Response {
        match &self.addr {
            BackendAddr::GoHandler(inner) => call_go_handler(inner.clone(), req, target_path).await,
            BackendAddr::Tcp(port)        => self.proxy_tcp(req, target_path, *port).await,
            BackendAddr::Unix(sock)       => self.proxy_unix(req, target_path, sock.clone()).await,
        }
    }

    async fn proxy_tcp(&self, req: Request, target_path: &str, port: u16) -> Response {
        let client = self.tcp_client.as_ref().unwrap();
        let uri = match build_uri(&format!("http://127.0.0.1:{port}"), target_path, req.uri()) {
            Ok(u)  => u,
            Err(e) => {
                warn!("failed to build upstream URI: {e}");
                return (StatusCode::INTERNAL_SERVER_ERROR, "bad gateway").into_response();
            }
        };
        let (parts, body) = req.into_parts();
        let mut upstream = hyper::Request::builder()
            .method(parts.method).uri(uri).version(hyper::Version::HTTP_11);
        for (k, v) in &parts.headers { upstream = upstream.header(k, v); }
        let upstream_req = match upstream.body(body) {
            Ok(r)  => r,
            Err(e) => {
                warn!("failed to build upstream request: {e}");
                return (StatusCode::INTERNAL_SERVER_ERROR, "bad gateway").into_response();
            }
        };
        match client.request(upstream_req).await {
            Ok(resp) => strip_cors(resp.map(Body::new)),
            Err(e)   => { warn!("upstream error: {e}"); (StatusCode::BAD_GATEWAY, "bad gateway").into_response() }
        }
    }

    async fn proxy_unix(&self, req: Request, target_path: &str, sock: PathBuf) -> Response {
        let stream = match UnixStream::connect(&sock).await {
            Ok(s)  => s,
            Err(e) => {
                warn!("failed to connect to backend socket {}: {e}", sock.display());
                return (StatusCode::BAD_GATEWAY, "backend unavailable").into_response();
            }
        };
        let (mut sender, conn) =
            match hyper::client::conn::http1::handshake(hyper_util::rt::TokioIo::new(stream)).await {
                Ok(p)  => p,
                Err(e) => {
                    warn!("HTTP handshake failed: {e}");
                    return (StatusCode::BAD_GATEWAY, "bad gateway").into_response();
                }
            };
        tokio::spawn(async move { if let Err(e) = conn.await { warn!("backend connection error: {e}"); } });

        let uri = match build_uri("http://localhost", target_path, req.uri()) {
            Ok(u)  => u,
            Err(e) => {
                warn!("failed to build upstream URI: {e}");
                return (StatusCode::INTERNAL_SERVER_ERROR, "bad gateway").into_response();
            }
        };
        let (parts, body) = req.into_parts();
        let body_bytes: Bytes = match body.collect().await {
            Ok(c)  => c.to_bytes(),
            Err(e) => {
                warn!("failed to read request body: {e}");
                return (StatusCode::INTERNAL_SERVER_ERROR, "bad gateway").into_response();
            }
        };
        let content_length = body_bytes.len();
        let mut upstream = hyper::Request::builder()
            .method(parts.method).uri(uri).version(hyper::Version::HTTP_11);
        for (k, v) in &parts.headers {
            if k != axum::http::header::CONTENT_LENGTH { upstream = upstream.header(k, v); }
        }
        upstream = upstream
            .header(axum::http::header::CONTENT_LENGTH, content_length)
            .header(axum::http::header::HOST, "localhost");
        let upstream_req = match upstream.body(Full::new(body_bytes)) {
            Ok(r)  => r,
            Err(e) => {
                warn!("failed to build upstream request: {e}");
                return (StatusCode::INTERNAL_SERVER_ERROR, "bad gateway").into_response();
            }
        };
        match sender.send_request(upstream_req).await {
            Ok(resp) => strip_cors(resp.map(|b: Incoming| Body::new(b))),
            Err(e)   => { warn!("upstream send error: {e}"); (StatusCode::BAD_GATEWAY, "bad gateway").into_response() }
        }
    }
}

// ── In-process Go handler dispatch (streaming) ───────────────────────────────

/// Dispatch a request directly to Go's http.Handler via the C callback.
///
/// Architecture:
/// - An mpsc channel (capacity 64) is created; the Sender is heap-allocated
///   and its raw pointer is written into DmrResponse.stream_ctx.
/// - Go's ServeHTTP runs on a spawn_blocking thread; every Write/Flush call
///   invokes dmr_write_chunk(), sending chunks into the channel.
/// - When ServeHTTP returns, Go calls dmr_close_stream(), dropping the Sender
///   and closing the channel.
/// - The Receiver is wrapped in a ReceiverStream and returned as the axum
///   response Body, so chunks flow to the client as they are produced.
async fn call_go_handler(
    inner:       Arc<GoHandlerInner>,
    req:         Request,
    target_path: &str,
) -> Response {
    // ── 1. Collect request body ───────────────────────────────────────────
    let method_str = req.method().to_string();
    let path_str   = target_path.to_owned();
    let headers    = req.headers().clone();

    let (_, body) = req.into_parts();
    let body_bytes: Bytes = match body.collect().await {
        Ok(c)  => c.to_bytes(),
        Err(e) => {
            warn!("failed to read request body: {e}");
            return (StatusCode::INTERNAL_SERVER_ERROR, "bad gateway").into_response();
        }
    };

    // ── 2. Serialise request headers ──────────────────────────────────────
    let mut hdr_block: Vec<u8> = Vec::new();
    for (name, value) in &headers {
        hdr_block.extend_from_slice(name.as_str().as_bytes());
        hdr_block.extend_from_slice(b": ");
        hdr_block.extend_from_slice(value.as_bytes());
        hdr_block.push(0);
    }
    hdr_block.push(0);

    // ── 3. Create streaming channel ───────────────────────────────────────
    // Capacity 64: enough to buffer several chunks so Go is rarely blocked.
    let (tx, rx) = mpsc::channel::<Option<Bytes>>(64);

    // Heap-allocate the sender; the raw pointer goes to Go via stream_ctx.
    // Go frees it by calling dmr_close_stream().
    let tx_ptr = Box::into_raw(Box::new(tx)) as *mut c_void;

    // ── 4. Launch Go handler on a blocking thread ─────────────────────────
    // We need the header/status before we can build the axum response, so
    // we use a oneshot channel to get that metadata back from the thread.
    let (meta_tx, meta_rx) = tokio::sync::oneshot::channel::<(u16, Vec<(String, String)>)>();

    let body_len  = body_bytes.len();
    let body_ptr  = { let mut v = body_bytes.to_vec(); let p = v.as_mut_ptr(); std::mem::forget(v); p };
    let hdr_len   = hdr_block.len();
    let hdr_ptr   = { let mut hb = hdr_block; let p = hb.as_mut_ptr(); std::mem::forget(hb); p };
    let method_c  = std::ffi::CString::new(method_str).unwrap_or_default();
    let path_c    = std::ffi::CString::new(path_str).unwrap_or_default();

    // Wrap raw pointers for Send across the thread boundary.
    // Both handler_ctx (Go cgo.Handle) and tx_ptr (Box<ChunkSender>) are
    // safe to send to another thread.
    struct SendPtr(*mut c_void);
    unsafe impl Send for SendPtr {}

    let ctx_send    = SendPtr(inner.handler_ctx);
    let tx_ptr_send = SendPtr(tx_ptr);
    // hdr_ptr and body_ptr also need wrapping since they're *mut u8.
    struct SendU8Ptr(*mut u8);
    unsafe impl Send for SendU8Ptr {}
    let hdr_ptr_send  = SendU8Ptr(hdr_ptr);
    let body_ptr_send = SendU8Ptr(body_ptr);

    let handler_fn = inner.handler_fn;

    // All raw pointers used inside the closure are safe to send:
    // - handler_ctx: Go cgo.Handle (uintptr_t)
    // - tx_ptr: Box<ChunkSender> allocated on this thread
    // - hdr_ptr / body_ptr: Vec allocations from this thread
    // - method_c / path_c: CString allocated on this thread
    // Wrap the closure in a SendClosure to assert this to the compiler.
    struct SendClosure<F: FnOnce()>(F);
    unsafe impl<F: FnOnce()> Send for SendClosure<F> {}
    impl<F: FnOnce()> SendClosure<F> {
        fn call(self) { (self.0)() }
    }

    let closure = SendClosure(move || {
        let handler_ctx = ctx_send.0;
        let stream_ctx  = tx_ptr_send.0;
        let hdr_ptr     = hdr_ptr_send.0;
        let body_ptr    = body_ptr_send.0;

        let req_ffi = DmrRequest {
            method:       method_c.as_ptr(),
            path:         path_c.as_ptr(),
            header_block: DmrBytes { ptr: hdr_ptr, len: hdr_len },
            body:         DmrBytes { ptr: body_ptr, len: body_len },
            _lifetime:    std::marker::PhantomData,
        };
        let mut resp_ffi = DmrResponse {
            status:       500,
            header_block: DmrBytes { ptr: std::ptr::null_mut(), len: 0 },
            stream_ctx,
        };

        // Call Go. Go calls dmr_write_chunk() for each chunk and
        // dmr_close_stream() when ServeHTTP returns.
        unsafe { handler_fn(handler_ctx, &req_ffi, &mut resp_ffi) };

        // Free request buffers.
        unsafe {
            dmr_free_bytes(DmrBytes { ptr: hdr_ptr, len: hdr_len });
            dmr_free_bytes(DmrBytes { ptr: body_ptr, len: body_len });
        }

        // Parse response headers and send metadata to the async side.
        let status = resp_ffi.status;
        let mut parsed_headers: Vec<(String, String)> = Vec::new();
        if !resp_ffi.header_block.ptr.is_null() && resp_ffi.header_block.len > 0 {
            let raw = unsafe {
                std::slice::from_raw_parts(resp_ffi.header_block.ptr, resp_ffi.header_block.len)
            };
            let mut pos = 0;
            while pos < raw.len() {
                let end = raw[pos..].iter().position(|&b| b == 0)
                    .map(|i| pos + i).unwrap_or(raw.len());
                if end == pos { break; }
                if let Ok(entry) = std::str::from_utf8(&raw[pos..end]) {
                    if let Some(sep) = entry.find(": ") {
                        parsed_headers.push((entry[..sep].to_owned(), entry[sep + 2..].to_owned()));
                    }
                }
                pos = end + 1;
            }
        }
        unsafe { dmr_free_bytes(resp_ffi.header_block); }

        // Ignore error: if receiver dropped, the client already disconnected.
        let _ = meta_tx.send((status, parsed_headers));
    });
    tokio::task::spawn_blocking(move || closure.call());

    // ── 5. Wait for status + headers, then stream body ────────────────────
    // Use a generous timeout so a misbehaving handler never blocks the
    // Tokio executor indefinitely.
    let (status_code, parsed_headers) = match tokio::time::timeout(
        std::time::Duration::from_secs(300),
        meta_rx,
    ).await {
        Ok(Ok(m)) => m,
        Ok(Err(_)) => {
            warn!("go handler thread dropped meta sender");
            return (StatusCode::INTERNAL_SERVER_ERROR, "handler error").into_response();
        }
        Err(_) => {
            warn!("go handler timed out waiting for response headers");
            return (StatusCode::GATEWAY_TIMEOUT, "handler timeout").into_response();
        }
    };

    let status = StatusCode::from_u16(status_code)
        .unwrap_or(StatusCode::INTERNAL_SERVER_ERROR);

    // Build a streaming Body from the mpsc receiver.
    // None items close the stream; Bytes items are forwarded as chunks.
    // Flatten Option<Bytes> items: None closes the stream, Some(b) yields b.
    let stream = ReceiverStream::new(rx)
        .take_while(|item: &Option<Bytes>| item.is_some())
        .map(|item: Option<Bytes>| Ok::<Bytes, std::convert::Infallible>(item.unwrap()));
    let streaming_body = Body::from_stream(stream);

    let mut builder = axum::http::response::Builder::new().status(status);
    for (name, value) in &parsed_headers {
        if let (Ok(n), Ok(v)) = (
            HeaderName::from_bytes(name.as_bytes()),
            HeaderValue::from_str(value),
        ) {
            builder = builder.header(n, v);
        }
    }

    builder.body(streaming_body).unwrap_or_else(|_| {
        (StatusCode::INTERNAL_SERVER_ERROR, "response build error").into_response()
    })
}

// ── Helpers ───────────────────────────────────────────────────────────────────

fn build_uri(base: &str, target_path: &str, original: &Uri) -> anyhow::Result<Uri> {
    let pq = match original.query() {
        Some(q) => format!("{target_path}?{q}"),
        None    => target_path.to_string(),
    };
    Ok(format!("{base}{pq}").parse::<Uri>()?)
}

fn strip_cors(resp: Response<Body>) -> Response {
    let (mut parts, body) = resp.into_parts();
    parts.headers.remove(axum::http::header::ACCESS_CONTROL_ALLOW_ORIGIN);
    Response::from_parts(parts, body)
}
