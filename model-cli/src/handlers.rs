use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;

use axum::body::Body;
use axum::extract::State;
use axum::http::header;
use axum::response::{IntoResponse, Json, Response};
use axum::routing::{get, post};
use axum::Router as AxumRouter;
use tokio_stream::StreamExt;
use tower_http::cors::CorsLayer;
use tower_http::trace::TraceLayer;

use crate::error::AppError;
use crate::router::Router;
use crate::types::{ChatCompletionRequest, EmbeddingRequest, HealthResponse};

/// Shared application state passed to every handler.
pub struct AppState {
    pub router: Router,
    pub master_key: Option<String>,
}

/// Build the axum application with all routes, CORS, and tracing layers.
///
/// This single definition is shared by both the standalone binary (`main.rs`)
/// and the CGo static-library entry point (`lib.rs`).
pub fn build_app(state: Arc<AppState>) -> AxumRouter {
    let auth_layer = axum::middleware::from_fn_with_state(
        state.master_key.clone(),
        crate::auth::auth_middleware,
    );

    let protected_routes = AxumRouter::new()
        .route("/v1/chat/completions", post(chat_completion_handler))
        .route("/chat/completions", post(chat_completion_handler))
        .route("/v1/embeddings", post(embeddings_handler))
        .route("/embeddings", post(embeddings_handler))
        .route("/v1/models", get(list_models_handler))
        .route("/models", get(list_models_handler))
        .layer(auth_layer);

    let public_routes = AxumRouter::new()
        .route("/health", get(health_handler))
        .route("/", get(health_handler));

    public_routes
        .merge(protected_routes)
        .layer(
            CorsLayer::new()
                .allow_origin(tower_http::cors::Any)
                .allow_methods([
                    axum::http::Method::GET,
                    axum::http::Method::POST,
                    axum::http::Method::OPTIONS,
                ])
                .allow_headers([
                    axum::http::header::CONTENT_TYPE,
                    axum::http::header::AUTHORIZATION,
                    axum::http::HeaderName::from_static("x-api-key"),
                ]),
        )
        .layer(TraceLayer::new_for_http())
        .with_state(state)
}

/// Core async gateway logic shared between the binary and the CGo library.
///
/// Loads config, builds the router and app, and serves until the process exits.
pub async fn run_gateway_async(config: PathBuf, host: String, port: u16, verbose: bool) {
    let log_filter = if verbose {
        "model_cli=debug,tower_http=debug"
    } else {
        "model_cli=info,tower_http=info"
    };

    dmr_common::init_tracing(log_filter);

    tracing::info!("Loading config from: {}", config.display());
    let cfg = match crate::config::Config::load(&config) {
        Ok(c) => c,
        Err(e) => {
            tracing::error!("Failed to load config: {}", e);
            std::process::exit(1);
        }
    };

    let model_count = cfg.model_list.len();
    let model_names: Vec<&str> = cfg.model_list.iter().map(|m| m.model_name.as_str()).collect();
    tracing::info!("Loaded {} model deployment(s): {:?}", model_count, model_names);

    let master_key = cfg.general_settings.master_key.clone();
    if master_key.is_some() {
        tracing::info!("Authentication enabled (master_key is set)");
    } else {
        tracing::warn!("No master_key configured — API is open to all requests");
    }

    let llm_router = match crate::router::Router::from_config(&cfg) {
        Ok(r) => r,
        Err(e) => {
            tracing::error!("Failed to build router: {}", e);
            std::process::exit(1);
        }
    };

    let state = Arc::new(AppState {
        router: llm_router,
        master_key: master_key.clone(),
    });

    let app = build_app(state);

    let addr: SocketAddr = format!("{}:{}", host, port)
        .parse()
        .expect("Invalid host:port");

    tracing::info!(
        "model-cli gateway v{} listening on {}",
        env!("CARGO_PKG_VERSION"),
        addr
    );
    tracing::info!("  Chat completions: http://{}/v1/chat/completions", addr);
    tracing::info!("  Models:           http://{}/v1/models", addr);
    tracing::info!("  Health:           http://{}/health", addr);

    let listener = tokio::net::TcpListener::bind(addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}

// ── Health check ──

pub async fn health_handler(State(state): State<Arc<AppState>>) -> Json<HealthResponse> {
    Json(HealthResponse {
        status: "healthy".to_string(),
        version: env!("CARGO_PKG_VERSION").to_string(),
        models: state.router.model_names(),
    })
}

// ── Model listing ──

pub async fn list_models_handler(State(state): State<Arc<AppState>>) -> impl IntoResponse {
    Json(state.router.list_models())
}

// ── Chat completions ──

pub async fn chat_completion_handler(
    State(state): State<Arc<AppState>>,
    Json(request): Json<ChatCompletionRequest>,
) -> Result<Response, AppError> {
    let is_stream = request.stream.unwrap_or(false);

    if is_stream {
        let byte_stream = state.router.chat_completion_stream(&request).await?;

        // Map stream errors into SSE-formatted error events without an extra task/channel.
        let body_stream = byte_stream.map(|chunk| match chunk {
            Ok(bytes) => Ok(bytes),
            Err(e) => {
                let error_resp =
                    crate::types::ErrorResponse::new(e.to_string(), "server_error", None);
                let json = serde_json::to_string(&error_resp).unwrap_or_default();
                Ok::<_, std::io::Error>(bytes::Bytes::from(format!("data: {}\n\n", json)))
            }
        });

        let body = Body::from_stream(body_stream);

        Ok(Response::builder()
            .header(header::CONTENT_TYPE, "text/event-stream")
            .header(header::CACHE_CONTROL, "no-cache")
            .header(header::CONNECTION, "keep-alive")
            .header(
                "x-model-cli-version",
                axum::http::HeaderValue::from_static(env!("CARGO_PKG_VERSION")),
            )
            .body(body)
            .unwrap())
    } else {
        let response = state.router.chat_completion(&request).await?;

        let mut resp = Json(response).into_response();
        resp.headers_mut().insert(
            "x-model-cli-version",
            axum::http::HeaderValue::from_static(env!("CARGO_PKG_VERSION")),
        );
        Ok(resp)
    }
}

// ── Embeddings ──

pub async fn embeddings_handler(
    State(state): State<Arc<AppState>>,
    Json(request): Json<EmbeddingRequest>,
) -> Result<Response, AppError> {
    let response = state.router.embedding(&request).await?;
    let mut resp = Json(response).into_response();
    resp.headers_mut().insert(
        "x-model-cli-version",
        axum::http::HeaderValue::from_static(env!("CARGO_PKG_VERSION")),
    );
    Ok(resp)
}
