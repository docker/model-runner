//! dmr-router: HTTP routing layer for Docker Model Runner, compiled as a
//! CGo-linked static library.
//!
//! Exposes two C functions to Go:
//!
//!   dmr_router_serve(cfg, handle_out) -> i32
//!     Blocks until the router shuts down. Returns 0 on clean shutdown,
//!     non-zero on error. Must be called from a dedicated goroutine.
//!
//!   dmr_router_stop(handle) -> void
//!     Signals the router to shut down gracefully and frees the handle.
//!     Safe to call from any goroutine.
//!
//!   dmr_free_bytes(b: DmrBytes) -> void
//!     Frees a byte buffer allocated on the Rust side of the FFI boundary.
//!     Go calls this to release DmrRequest.header_block and DmrRequest.body
//!     after it has finished reading them.

mod cors;
mod proxy;
mod routes;

use std::ffi::CStr;
use std::net::SocketAddr;
use std::os::raw::{c_char, c_int, c_void};
use std::path::PathBuf;

use tokio::sync::oneshot;
use tracing::info;

use crate::proxy::{BackendClient, DmrBytes, DmrHandlerFn};
use crate::routes::build_router;

// ── Default CORS origins ─────────────────────────────────────────────────────

/// Origins always added to the CORS allow-list, matching
/// Go's envconfig.AllowedOrigins() baseline.
const DEFAULT_ORIGINS: &[&str] = &[
    "http://localhost",
    "http://127.0.0.1",
    "http://0.0.0.0",
];

// ── C-facing configuration struct ───────────────────────────────────────────

/// Configuration passed from Go to `dmr_router_serve`.
/// All string fields are NUL-terminated C strings owned by the caller; they
/// must remain valid for the duration of the `dmr_router_serve` call.
#[repr(C)]
pub struct DmrRouterConfig {
    /// NUL-terminated path of the Unix socket the router listens on.
    /// Pass NULL to use a TCP port instead.
    pub listen_sock: *const c_char,
    /// TCP port the router listens on. Ignored when listen_sock is non-NULL.
    pub listen_port: u16,

    /// In-process Go handler (no network hop).
    /// When non-NULL, backend_sock and backend_port are ignored.
    pub handler_fn:  Option<DmrHandlerFn>,
    pub handler_ctx: *mut c_void,

    /// Unix socket path of the Go backend (used when handler_fn is NULL).
    pub backend_sock: *const c_char,
    /// TCP port of the Go backend (used when handler_fn is NULL).
    pub backend_port: u16,

    /// NUL-terminated comma-separated allowed CORS origins.  May be NULL.
    pub allowed_origins: *const c_char,
    /// NUL-terminated version string served at GET /version.  May be NULL.
    pub version: *const c_char,
}

/// Opaque stop handle returned to Go so it can call `dmr_router_stop`.
pub struct DmrRouterHandle {
    tx: Option<oneshot::Sender<()>>,
}

// ── Parsed (safe-Rust) configuration ────────────────────────────────────────

/// A network address: either a Unix domain socket path or a TCP port.
enum Addr {
    Unix(PathBuf),
    Tcp(u16),
}

struct Config {
    listen:          Addr,
    backend:         BackendClient,
    allowed_origins: Vec<String>,
    version:         String,
}

// ── C string helpers ─────────────────────────────────────────────────────────

unsafe fn cstr_to_string(ptr: *const c_char) -> Option<String> {
    if ptr.is_null() {
        None
    } else {
        Some(unsafe { CStr::from_ptr(ptr) }.to_string_lossy().into_owned())
    }
}

unsafe fn parse_addr(sock_ptr: *const c_char, port: u16) -> Addr {
    match unsafe { cstr_to_string(sock_ptr) } {
        Some(path) => Addr::Unix(PathBuf::from(path)),
        None => Addr::Tcp(port),
    }
}

/// Parse a `DmrRouterConfig` into a safe `Config`.
///
/// # Safety
/// All pointer fields in `cfg` must be valid NUL-terminated C strings or NULL.
unsafe fn parse_config(cfg: &DmrRouterConfig) -> Config {
    let listen = unsafe { parse_addr(cfg.listen_sock, cfg.listen_port) };

    let backend = if let Some(f) = cfg.handler_fn {
        // In-process mode: call Go directly, no socket.
        unsafe { BackendClient::new_go(f, cfg.handler_ctx) }
    } else {
        match unsafe { cstr_to_string(cfg.backend_sock) } {
            Some(path) => BackendClient::new_unix(PathBuf::from(path)),
            None       => BackendClient::new_tcp(cfg.backend_port),
        }
    };

    let mut allowed_origins: Vec<String> =
        DEFAULT_ORIGINS.iter().map(|s| s.to_string()).collect();
    if let Some(raw) = unsafe { cstr_to_string(cfg.allowed_origins) } {
        for o in raw.split(',') {
            let o = o.trim().to_string();
            if !o.is_empty() {
                allowed_origins.push(o);
            }
        }
    }

    let version = unsafe { cstr_to_string(cfg.version) }
        .unwrap_or_else(|| "unknown".to_string());

    Config { listen, backend, allowed_origins, version }
}

// ── Public C API ─────────────────────────────────────────────────────────────

/// Allocate a new `DmrRouterHandle` and return it to Go.
///
/// Go calls this **before** spawning the `dmr_router_serve` goroutine so that
/// it holds a valid stop handle from the very start — before `block_on` ever
/// runs.  The same handle pointer is then passed to `dmr_router_serve` via
/// `handle_out` so Rust can write the `oneshot::Sender` into it.
///
/// The returned pointer must be freed exactly once, either by `dmr_router_stop`
/// or by `dmr_router_free_handle` if the router never started.
#[unsafe(no_mangle)]
pub extern "C" fn dmr_router_new_handle() -> *mut DmrRouterHandle {
    Box::into_raw(Box::new(DmrRouterHandle { tx: None }))
}

/// Free a `DmrRouterHandle` that was never passed to `dmr_router_serve`.
/// Use `dmr_router_stop` for handles that were passed to `dmr_router_serve`.
///
/// # Safety
/// `handle` must be a pointer obtained from `dmr_router_new_handle` that has
/// not yet been passed to `dmr_router_serve` and has not been freed already.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dmr_router_free_handle(handle: *mut DmrRouterHandle) {
    if !handle.is_null() {
        drop(unsafe { Box::from_raw(handle) });
    }
}

/// Free a `DmrBytes` buffer that was allocated by the Rust side of the FFI
/// boundary.  Go calls this after reading `DmrRequest.header_block` and
/// `DmrRequest.body`.
///
/// # Safety
/// `b.ptr` must be a pointer previously allocated by the Rust global allocator
/// (i.e. from a `Vec<u8>` via `Vec::into_raw_parts`), or NULL.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dmr_free_bytes(b: DmrBytes) {
    if !b.ptr.is_null() && b.len > 0 {
        drop(unsafe { Vec::from_raw_parts(b.ptr, b.len, b.len) });
    }
}

/// Start the router and block until `dmr_router_stop` is called or a fatal
/// error occurs.  Returns 0 on clean shutdown, 1 on error.
///
/// # Safety
/// `cfg` must point to a valid `DmrRouterConfig`. All pointer fields inside
/// `cfg` must be valid NUL-terminated C strings for the duration of this call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dmr_router_serve(
    cfg: *const DmrRouterConfig,
    handle_out: *mut *mut DmrRouterHandle,
) -> c_int {
    if cfg.is_null() {
        return 1;
    }
    let config = unsafe { parse_config(&*cfg) };

    dmr_common::init_tracing("info");

    let rt = match tokio::runtime::Runtime::new() {
        Ok(r) => r,
        Err(e) => {
            eprintln!("dmr-router: failed to create tokio runtime: {e}");
            return 1;
        }
    };

    let (stop_tx, stop_rx) = oneshot::channel::<()>();

    // Wire the stop sender into the pre-allocated handle.
    // Go calls dmr_router_new_handle() before starting this function and
    // passes the result as *handle_out; we write the tx field into the
    // existing allocation rather than replacing the pointer.
    if !handle_out.is_null() {
        let h: *mut DmrRouterHandle = unsafe { *handle_out };
        if !h.is_null() {
            unsafe { (*h).tx = Some(stop_tx) };
        }
        // If *handle_out is null (legacy / direct call), allocate a new handle.
        else {
            let handle = Box::new(DmrRouterHandle { tx: Some(stop_tx) });
            unsafe { *handle_out = Box::into_raw(handle) };
        }
    }

    let result = rt.block_on(serve(config, stop_rx));
    match result {
        Ok(()) => 0,
        Err(e) => {
            eprintln!("dmr-router: {e}");
            1
        }
    }
}

/// Signal the router to shut down gracefully and free the handle.
///
/// # Safety
/// `handle` must be a pointer previously obtained from `dmr_router_serve`
/// via `handle_out`, and must not have been freed already.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dmr_router_stop(handle: *mut DmrRouterHandle) {
    if handle.is_null() {
        return;
    }
    let mut h = unsafe { Box::from_raw(handle) };
    if let Some(tx) = h.tx.take() {
        let _ = tx.send(());
    }
}

// ── Async server core ────────────────────────────────────────────────────────

async fn serve(cfg: Config, stop_rx: oneshot::Receiver<()>) -> anyhow::Result<()> {
    let app = build_router(cfg.backend, cfg.allowed_origins, cfg.version);

    let shutdown = async move {
        let _ = stop_rx.await;
        info!("dmr-router: shutdown signal received");
    };

    match cfg.listen {
        Addr::Tcp(port) => {
            let addr: SocketAddr = format!("0.0.0.0:{port}").parse()?;
            info!(%addr, "dmr-router listening on TCP");
            let listener = tokio::net::TcpListener::bind(addr).await?;
            axum::serve(listener, app)
                .with_graceful_shutdown(shutdown)
                .await?;
        }
        Addr::Unix(ref path) => {
            let _ = std::fs::remove_file(path);
            let listener = tokio::net::UnixListener::bind(path)?;
            info!(path = %path.display(), "dmr-router listening on Unix socket");
            axum::serve(listener, app)
                .with_graceful_shutdown(shutdown)
                .await?;
        }
    }

    Ok(())
}
