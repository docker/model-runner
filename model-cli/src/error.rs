use std::fmt;

use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;

use crate::types::ErrorResponse;

#[derive(Debug)]
pub enum AppError {
    /// Model not found in the configured model list.
    ModelNotFound(String),
    /// Authentication failure.
    Unauthorized(String),
    /// Provider returned an error.
    ProviderError { status: StatusCode, message: String },
    /// All deployments for a model failed.
    AllDeploymentsFailed(String),
    /// Invalid request body / parameters.
    BadRequest(String),
    /// Internal server error.
    Internal(String),
}

impl fmt::Display for AppError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            AppError::ModelNotFound(model) => write!(f, "Model '{}' not found", model),
            AppError::Unauthorized(msg) => write!(f, "Unauthorized: {}", msg),
            AppError::ProviderError { message, .. } => write!(f, "Provider error: {}", message),
            AppError::AllDeploymentsFailed(model) => {
                write!(f, "All deployments failed for model '{}'", model)
            }
            AppError::BadRequest(msg) => write!(f, "Bad request: {}", msg),
            AppError::Internal(msg) => write!(f, "Internal error: {}", msg),
        }
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> Response {
        let (status, error_type, message) = match &self {
            AppError::ModelNotFound(model) => (
                StatusCode::NOT_FOUND,
                "invalid_request_error",
                format!(
                    "The model '{}' does not exist or you do not have access to it.",
                    model
                ),
            ),
            AppError::Unauthorized(msg) => (
                StatusCode::UNAUTHORIZED,
                "authentication_error",
                msg.clone(),
            ),
            AppError::ProviderError { status, message } => {
                (*status, "upstream_error", message.clone())
            }
            AppError::AllDeploymentsFailed(model) => (
                StatusCode::SERVICE_UNAVAILABLE,
                "server_error",
                format!("All deployments for model '{}' are unavailable.", model),
            ),
            AppError::BadRequest(msg) => (
                StatusCode::BAD_REQUEST,
                "invalid_request_error",
                msg.clone(),
            ),
            AppError::Internal(msg) => (
                StatusCode::INTERNAL_SERVER_ERROR,
                "server_error",
                msg.clone(),
            ),
        };

        let body = ErrorResponse::new(message, error_type, None);
        (status, Json(body)).into_response()
    }
}
