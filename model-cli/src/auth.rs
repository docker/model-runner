use axum::extract::{Request, State};
use axum::middleware::Next;
use axum::response::Response;
use subtle::ConstantTimeEq;

use crate::error::AppError;

/// Extract and validate the API key from the Authorization header.
/// If no master_key is configured, all requests are allowed.
pub async fn auth_middleware(
    State(master_key): State<Option<String>>,
    request: Request,
    next: Next,
) -> Result<Response, AppError> {
    if let Some(ref expected_key) = master_key {
        let provided_key = request
            .headers()
            .get("authorization")
            .and_then(|v| v.to_str().ok())
            .map(|h| h.strip_prefix("Bearer ").unwrap_or(h))
            .or_else(|| {
                request
                    .headers()
                    .get("x-api-key")
                    .and_then(|v| v.to_str().ok())
            })
            .unwrap_or("");

        // Use constant-time comparison to prevent timing attacks.
        if provided_key
            .as_bytes()
            .ct_eq(expected_key.as_bytes())
            .unwrap_u8()
            == 0
        {
            return Err(AppError::Unauthorized("Invalid API key".to_string()));
        }
    }

    Ok(next.run(request).await)
}
