mod auth;
mod config;
mod error;
mod handlers;
mod providers;
mod router;
mod types;

use std::ffi::CStr;
use std::os::raw::{c_char, c_int};
use std::path::PathBuf;

use axum::routing::{get, post};
use axum::Router as AxumRouter;
use tower_http::cors::CorsLayer;
use tower_http::trace::TraceLayer;
use tracing_subscriber::EnvFilter;

use handlers::AppState;

/// C-callable entry point invoked by the Go CLI's `gateway` subcommand.
///
/// # Safety
/// `argv` must be a valid array of `argc` non-null, null-terminated C strings
/// that remains valid for the duration of this call.
#[no_mangle]
pub unsafe extern "C" fn run_gateway(argc: c_int, argv: *const *const c_char) -> c_int {
    // Rebuild the argument list from the C array.
    // argv[0] is the program name (ignored by clap when we parse manually);
    // the remaining elements are the gateway flags/values.
    let args: Vec<String> = (0..argc as usize)
        .filter_map(|i| {
            let ptr = *argv.add(i);
            if ptr.is_null() {
                None
            } else {
                CStr::from_ptr(ptr).to_str().ok().map(|s| s.to_owned())
            }
        })
        .collect();

    // Parse flags directly (skip argv[0] = program name).
    // Expected layout: ["model-cli", "--config", "<path>", ...]
    let mut config: Option<PathBuf> = None;
    let mut host = "0.0.0.0".to_string();
    let mut port: u16 = 4000;
    let mut verbose = false;

    let mut it = args.iter().skip(1).peekable();
    while let Some(arg) = it.next() {
        match arg.as_str() {
            "-c" | "--config" => {
                if let Some(val) = it.next() {
                    config = Some(PathBuf::from(val));
                }
            }
            "--host" => {
                if let Some(val) = it.next() {
                    host = val.clone();
                }
            }
            "-p" | "--port" => {
                if let Some(val) = it.next() {
                    if let Ok(n) = val.parse::<u16>() {
                        port = n;
                    }
                }
            }
            "-v" | "--verbose" => verbose = true,
            "--help" | "-h" => {
                eprintln!(concat!(
                    "Usage: model-cli gateway [OPTIONS] --config <CONFIG>\n",
                    "\n",
                    "Options:\n",
                    "  -c, --config <CONFIG>   Path to the YAML configuration file\n",
                    "      --host <HOST>        Host address [default: 0.0.0.0]\n",
                    "  -p, --port <PORT>        Port [default: 4000]\n",
                    "  -v, --verbose            Enable debug logging\n",
                    "  -h, --help               Print help",
                ));
                return 0;
            }
            _ => {}
        }
    }

    let config = match config {
        Some(p) => p,
        None => {
            eprintln!("error: --config is required");
            return 1;
        }
    };

    let rt = match tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
    {
        Ok(r) => r,
        Err(e) => {
            eprintln!("error: failed to build tokio runtime: {e}");
            return 1;
        }
    };

    rt.block_on(async_run_gateway(config, host, port, verbose));
    0
}

async fn async_run_gateway(config: PathBuf, host: String, port: u16, verbose: bool) {
    use std::net::SocketAddr;
    use std::sync::Arc;

    let log_filter = if verbose {
        "model_cli=debug,tower_http=debug"
    } else {
        "model_cli=info,tower_http=info"
    };

    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new(log_filter)),
        )
        .init();

    tracing::info!("Loading config from: {}", config.display());
    let cfg = match config::Config::load(&config) {
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

    let llm_router = match router::Router::from_config(&cfg) {
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

fn build_app(state: std::sync::Arc<AppState>) -> AxumRouter {
    let auth_layer = axum::middleware::from_fn_with_state(
        state.master_key.clone(),
        auth::auth_middleware,
    );

    let protected_routes = AxumRouter::new()
        .route(
            "/v1/chat/completions",
            post(handlers::chat_completion_handler),
        )
        .route(
            "/chat/completions",
            post(handlers::chat_completion_handler),
        )
        .route("/v1/embeddings", post(handlers::embeddings_handler))
        .route("/embeddings", post(handlers::embeddings_handler))
        .route("/v1/models", get(handlers::list_models_handler))
        .route("/models", get(handlers::list_models_handler))
        .layer(auth_layer);

    let public_routes = AxumRouter::new()
        .route("/health", get(handlers::health_handler))
        .route("/", get(handlers::health_handler));

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
