//! CORS middleware matching the Go CorsMiddleware behaviour.
//!
//! Rules (from pkg/middleware/cors.go):
//! - If allowedOrigins contains "*", all origins are allowed.
//! - Otherwise, only origins in the allowed set receive CORS headers.
//! - Requests from origins not in the allowed set receive 403.
//! - Requests with no Origin header pass through unchanged.
//! - OPTIONS preflight: if origin is valid, respond 204 with CORS headers;
//!   otherwise fall through to the next handler.

use std::collections::HashSet;
use std::sync::Arc;
use std::task::{Context, Poll};

use axum::body::Body;
use axum::http::{header, Method, Request, Response, StatusCode};
use tower::{Layer, Service};

/// Builds a CORS layer from an origin allow-list.
#[derive(Clone)]
pub struct CorsLayer {
    allow_all: bool,
    allowed: Arc<HashSet<String>>,
}

impl CorsLayer {
    pub fn new(origins: Vec<String>) -> Self {
        let allow_all = origins.iter().any(|o| o == "*");
        let allowed = Arc::new(origins.into_iter().collect());
        Self { allow_all, allowed }
    }
}

impl<S> Layer<S> for CorsLayer {
    type Service = CorsMiddleware<S>;
    fn layer(&self, inner: S) -> Self::Service {
        CorsMiddleware {
            inner,
            allow_all: self.allow_all,
            allowed: self.allowed.clone(),
        }
    }
}

#[derive(Clone)]
pub struct CorsMiddleware<S> {
    inner: S,
    allow_all: bool,
    allowed: Arc<HashSet<String>>,
}

impl<S> Service<Request<Body>> for CorsMiddleware<S>
where
    S: Service<Request<Body>, Response = Response<Body>> + Clone + Send + 'static,
    S::Future: Send + 'static,
{
    type Response = Response<Body>;
    type Error = S::Error;
    type Future = std::pin::Pin<
        Box<dyn std::future::Future<Output = Result<Self::Response, Self::Error>> + Send>,
    >;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        self.inner.poll_ready(cx)
    }

    fn call(&mut self, req: Request<Body>) -> Self::Future {
        let allow_all = self.allow_all;
        let allowed = self.allowed.clone();
        let mut inner = self.inner.clone();

        Box::pin(async move {
            let origin = req
                .headers()
                .get(header::ORIGIN)
                .and_then(|v| v.to_str().ok())
                .map(|s| s.to_string());

            let origin_allowed = match &origin {
                None => true, // no Origin header → pass through
                Some(o) => allow_all || allowed.contains(o.as_str()),
            };

            if let Some(ref o) = origin {
                if !origin_allowed {
                    let mut resp = Response::new(Body::from("Origin not allowed"));
                    *resp.status_mut() = StatusCode::FORBIDDEN;
                    return Ok(resp);
                }

                // Handle OPTIONS preflight.
                if req.method() == Method::OPTIONS {
                    let mut resp = Response::new(Body::empty());
                    *resp.status_mut() = StatusCode::NO_CONTENT;
                    let h = resp.headers_mut();
                    h.insert(
                        header::ACCESS_CONTROL_ALLOW_ORIGIN,
                        o.parse().unwrap(),
                    );
                    h.insert(
                        header::ACCESS_CONTROL_ALLOW_CREDENTIALS,
                        "true".parse().unwrap(),
                    );
                    h.insert(
                        header::ACCESS_CONTROL_ALLOW_METHODS,
                        "GET, POST, DELETE".parse().unwrap(),
                    );
                    h.insert(
                        header::ACCESS_CONTROL_ALLOW_HEADERS,
                        "*".parse().unwrap(),
                    );
                    return Ok(resp);
                }
            }

            // Pass through to inner handler.
            let mut resp = inner.call(req).await?;

            // Attach Access-Control-Allow-Origin for non-preflight allowed origins.
            if let Some(o) = origin {
                if origin_allowed {
                    resp.headers_mut().insert(
                        header::ACCESS_CONTROL_ALLOW_ORIGIN,
                        o.parse().unwrap(),
                    );
                }
            }

            Ok(resp)
        })
    }
}
