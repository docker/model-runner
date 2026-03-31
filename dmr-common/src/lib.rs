//! Shared utilities for Docker Model Runner Rust crates.

/// Return the current time as seconds since the Unix epoch.
///
/// Used when constructing OpenAI-format response objects that require a
/// `created` timestamp.  Returns 0 on the (extremely unlikely) event that
/// the system clock predates 1970.
pub fn unix_now_secs() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

/// Initialise the global tracing subscriber.
///
/// Reads `RUST_LOG` from the environment; falls back to `fallback` if unset
/// or invalid.  Silently ignores subsequent calls (e.g. when called from both
/// a library entry point and a binary entry point in the same process).
///
/// # Arguments
/// * `fallback` – default filter string, e.g. `"info"` or `"myapp=debug"`.
pub fn init_tracing(fallback: &str) {
    let _ = tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new(fallback)),
        )
        .try_init();
}
