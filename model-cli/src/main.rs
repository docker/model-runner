mod auth;
mod config;
mod error;
mod handlers;
mod providers;
mod router;
mod types;

use std::path::PathBuf;

use clap::{Parser, Subcommand};

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
    handlers::run_gateway_async(args.config, args.host, args.port, args.verbose).await;
}
