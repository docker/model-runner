use std::sync::Arc;

use axum::body::Body;
use axum::extract::State;
use axum::http::header;
use axum::response::{IntoResponse, Json, Response};
use tokio_stream::StreamExt;

use crate::error::AppError;
use crate::router::Router;
use crate::types::{ChatCompletionRequest, EmbeddingRequest, HealthResponse};

/// Shared application state passed to every handler.
pub struct AppState {
    pub router: Router,
    pub master_key: Option<String>,
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
