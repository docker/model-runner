//! Route table mirroring pkg/routing/router.go.
//!
//! Design:
//! - All inference routes proxy to the Go backend unchanged.
//! - Alias routes (/v1/, /rerank, /score, /tokenize, /detokenize) prepend
//!   "/engines" to the path before proxying (mirrors AliasHandler).
//! - Path normalisation (double-slash collapsing) is applied globally via
//!   tower-http NormalizePath middleware.
//! - CORS is applied globally via our custom CorsLayer.

use axum::extract::{Request, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::routing::{any, get};
use axum::Router;
use tower::ServiceBuilder;
use tower_http::normalize_path::NormalizePathLayer;

use crate::cors::CorsLayer;
use crate::proxy::BackendClient;

/// Shared application state.
#[derive(Clone)]
struct AppState {
    backend: BackendClient,
    version: String,
}

/// Build the full axum Router.
pub fn build_router(
    backend: BackendClient,
    allowed_origins: Vec<String>,
    version: String,
) -> Router {
    let state = AppState { backend, version };

    let router = Router::new()
        // ── Static / informational routes ──────────────────────────────────
        .route("/", get(handle_health))
        .route("/version", get(handle_version))
        // ── Model management ───────────────────────────────────────────────
        .route("/models", any(proxy_direct))
        .route("/models/{*path}", any(proxy_direct))
        // ── Inference engine (direct) ──────────────────────────────────────
        .route("/engines/{*path}", any(proxy_direct))
        // ── Path aliases → prepend /engines ───────────────────────────────
        // These mirror the deleted AliasHandler in pkg/middleware/alias.go.
        .route("/v1/{*path}", any(proxy_alias))
        .route("/rerank", any(proxy_alias))
        .route("/score", any(proxy_alias))
        .route("/tokenize", any(proxy_alias))
        .route("/detokenize", any(proxy_alias))
        // ── Ollama compatibility layer (/api/) ─────────────────────────────
        .route("/api/{*path}", any(proxy_direct))
        // ── Anthropic compatibility layer (/anthropic/) ────────────────────
        .route("/anthropic/{*path}", any(proxy_direct))
        // ── OpenAI Responses API ───────────────────────────────────────────
        .route("/responses", any(proxy_direct))
        .route("/responses/{*path}", any(proxy_direct))
        .route("/v1/responses", any(proxy_direct))
        .route("/v1/responses/{*path}", any(proxy_direct))
        .route("/engines/responses", any(proxy_direct))
        .route("/engines/responses/{*path}", any(proxy_direct))
        // ── Observability ──────────────────────────────────────────────────
        .route("/logs", any(proxy_direct))
        .route("/metrics", any(proxy_direct))
        .with_state(state);

    // Apply path normalisation and CORS as outer layers.
    let cors = CorsLayer::new(allowed_origins);
    let svc = ServiceBuilder::new()
        .layer(NormalizePathLayer::trim_trailing_slash())
        .layer(cors);

    router.layer(svc)
}

// ── Handlers ────────────────────────────────────────────────────────────────

/// GET / → "Docker Model Runner is running"
async fn handle_health() -> impl IntoResponse {
    (StatusCode::OK, "Docker Model Runner is running")
}

/// GET /version → {"version":"<version>"}
async fn handle_version(State(state): State<AppState>) -> impl IntoResponse {
    let body = format!(r#"{{"version":"{}"}}"#, state.version);
    (StatusCode::OK, [("content-type", "application/json")], body)
}

/// Proxy request to the backend with the path unchanged.
async fn proxy_direct(State(state): State<AppState>, req: Request) -> Response {
    let path = req.uri().path_and_query().map_or_else(
        || req.uri().path().to_owned(),
        |pq| pq.as_str().to_owned(),
    );
    state.backend.proxy(req, &path).await
}

/// Alias handler: prepend "/engines" to the path, then proxy.
/// Mirrors the deleted pkg/middleware/alias.go AliasHandler.
async fn proxy_alias(State(state): State<AppState>, req: Request) -> Response {
    let original = req.uri().path_and_query().map_or_else(
        || req.uri().path().to_owned(),
        |pq| pq.as_str().to_owned(),
    );
    let aliased = format!("/engines{original}");
    state.backend.proxy(req, &aliased).await
}
