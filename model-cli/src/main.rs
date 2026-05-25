mod auth;
mod config;
mod error;
mod handlers;
mod providers;
mod router;
mod types;

use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;

use axum::routing::{get, post};
use axum::Router as AxumRouter;
use clap::{Parser, Subcommand};
use tower_http::cors::CorsLayer;
use tower_http::trace::TraceLayer;
use tracing_subscriber::EnvFilter;

use handlers::AppState;

/// model-cli: CLI tool for Docker Model Runner and compatible LLM providers.
#[derive(Parser, Debug)]
#[command(name = "model-cli", version, about)]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand, Debug)]
enum Command {
    /// Run an OpenAI-compatible LLM gateway that routes requests to configured providers.
    ///
    /// Supported providers include Docker Model Runner, Ollama, OpenAI, Anthropic,
    /// Groq, Mistral, Azure OpenAI, and many more OpenAI-compatible endpoints.
    Gateway(GatewayArgs),
}

#[derive(Parser, Debug)]
struct GatewayArgs {
    /// Path to the YAML configuration file.
    #[arg(short, long)]
    config: PathBuf,

    /// Host address to bind to.
    #[arg(long, default_value = "0.0.0.0")]
    host: String,

    /// Port to listen on.
    #[arg(short, long, default_value_t = 4000)]
    port: u16,

    /// Enable verbose (debug) logging.
    #[arg(short, long)]
    verbose: bool,
}

#[tokio::main]
async fn main() {
    let cli = Cli::parse();

    match cli.command {
        Command::Gateway(args) => run_gateway(args).await,
    }
}

async fn run_gateway(args: GatewayArgs) {
    let log_filter = if args.verbose {
        "model_cli=debug,tower_http=debug"
    } else {
        "model_cli=info,tower_http=info"
    };

    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new(log_filter)),
        )
        .init();

    tracing::info!("Loading config from: {}", args.config.display());
    let cfg = match config::Config::load(&args.config) {
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

    let addr: SocketAddr = format!("{}:{}", args.host, args.port)
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

fn build_app(state: Arc<AppState>) -> AxumRouter {
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
