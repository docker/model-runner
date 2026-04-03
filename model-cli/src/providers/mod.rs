pub mod anthropic;
pub mod openai;

use axum::http::StatusCode;
use bytes::Bytes;
use futures::Stream;
use std::pin::Pin;

use crate::config::ModelParams;
use crate::error::AppError;
use crate::types::{ChatCompletionRequest, ChatCompletionResponse, EmbeddingRequest, EmbeddingResponse};

/// Send `req`, check the HTTP status, and surface upstream errors as `AppError`.
///
/// `provider_label` appears in the network-failure message (e.g. `"provider"`
/// or `"Anthropic"`).  Returns the raw `reqwest::Response` on success.
///
/// Both `OpenAIProvider` and `AnthropicProvider` previously contained
/// identical copies of this logic as inherent methods.
pub async fn send_and_check(
    req: reqwest::RequestBuilder,
    provider_label: &str,
) -> Result<reqwest::Response, AppError> {
    let response = req.send().await.map_err(|e| AppError::ProviderError {
        status: StatusCode::BAD_GATEWAY,
        message: format!("Failed to reach {provider_label}: {e}"),
    })?;

    let status = response.status();
    if !status.is_success() {
        let body = response.text().await.unwrap_or_default();
        return Err(parse_upstream_error(
            StatusCode::from_u16(status.as_u16()).unwrap_or(StatusCode::BAD_GATEWAY),
            &body,
        ));
    }

    Ok(response)
}

/// A boxed stream of bytes (SSE chunks) or errors.
pub type ByteStream = Pin<Box<dyn Stream<Item = Result<Bytes, AppError>> + Send>>;

/// Trait that every LLM provider must implement.
#[async_trait::async_trait]
pub trait Provider: Send + Sync {
    /// Perform a non-streaming chat completion.
    async fn chat_completion(
        &self,
        request: &ChatCompletionRequest,
        params: &ModelParams,
    ) -> Result<ChatCompletionResponse, AppError>;

    /// Perform a streaming chat completion, returning an SSE byte stream.
    async fn chat_completion_stream(
        &self,
        request: &ChatCompletionRequest,
        params: &ModelParams,
    ) -> Result<ByteStream, AppError>;

    /// Perform an embedding request.
    async fn embedding(
        &self,
        request: &EmbeddingRequest,
        params: &ModelParams,
    ) -> Result<EmbeddingResponse, AppError>;
}

/// Resolve the correct provider implementation for a given provider name.
pub fn resolve_provider(provider_name: &str) -> Result<Box<dyn Provider>, AppError> {
    match provider_name {
        "openai"
        | "openai_like"
        | "together_ai"
        | "groq"
        | "deepseek"
        | "fireworks_ai"
        | "mistral"
        | "openrouter"
        | "perplexity"
        | "anyscale"
        | "deepinfra"
        | "xai"
        | "nvidia_nim"
        | "cerebras"
        | "sambanova"
        | "ollama"
        | "vllm"
        | "lm_studio"
        | "huggingface"
        | "docker_model_runner" => Ok(Box::new(openai::OpenAIProvider::new())),
        "anthropic" => Ok(Box::new(anthropic::AnthropicProvider::new())),
        "azure" | "azure_ai" => Ok(Box::new(openai::OpenAIProvider::new())),
        _ => {
            // Fall back to OpenAI-compatible for unknown providers
            tracing::warn!(
                "Unknown provider '{}', falling back to OpenAI-compatible",
                provider_name
            );
            Ok(Box::new(openai::OpenAIProvider::new()))
        }
    }
}

/// Build the complete API URL for a provider.
pub fn build_api_url(provider_name: &str, params: &ModelParams, endpoint: &str) -> String {
    if let Some(ref base) = params.api_base {
        let base = base.trim_end_matches('/');
        return format!("{}{}", base, endpoint);
    }

    match provider_name {
        "openai" => format!("https://api.openai.com/v1{}", endpoint),
        "anthropic" => format!("https://api.anthropic.com/v1{}", endpoint),
        "together_ai" => format!("https://api.together.xyz/v1{}", endpoint),
        "groq" => format!("https://api.groq.com/openai/v1{}", endpoint),
        "deepseek" => format!("https://api.deepseek.com/v1{}", endpoint),
        "fireworks_ai" => format!("https://api.fireworks.ai/inference/v1{}", endpoint),
        "mistral" => format!("https://api.mistral.ai/v1{}", endpoint),
        "openrouter" => format!("https://openrouter.ai/api/v1{}", endpoint),
        "perplexity" => format!("https://api.perplexity.ai{}", endpoint),
        "xai" => format!("https://api.x.ai/v1{}", endpoint),
        "nvidia_nim" => format!("https://integrate.api.nvidia.com/v1{}", endpoint),
        "cerebras" => format!("https://api.cerebras.ai/v1{}", endpoint),
        "sambanova" => format!("https://api.sambanova.ai/v1{}", endpoint),
        "deepinfra" => format!("https://api.deepinfra.com/v1/openai{}", endpoint),
        "ollama" => format!("http://localhost:11434/v1{}", endpoint),
        "vllm" => format!("http://localhost:8000/v1{}", endpoint),
        "lm_studio" => format!("http://localhost:1234/v1{}", endpoint),
        // Docker Model Runner exposes an OpenAI-compatible API at this base URL.
        // The engine path segment (engines/llama.cpp) is the default; users can
        // override via api_base if they target a different engine.
        "docker_model_runner" => {
            format!("http://localhost:12434/engines/llama.cpp/v1{}", endpoint)
        }
        "azure" | "azure_ai" => {
            // Azure requires api_base to be set; this is a compile-time fallback.
            format!(
                "https://YOUR_RESOURCE.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT{}",
                endpoint
            )
        }
        _ => format!("https://api.openai.com/v1{}", endpoint),
    }
}

/// Resolve the API key, checking params first, then environment variables.
pub fn resolve_api_key(provider_name: &str, params: &ModelParams) -> Option<String> {
    if let Some(ref key) = params.api_key {
        if !key.is_empty() {
            return Some(key.clone());
        }
    }

    // Check standard environment variables
    let env_var = match provider_name {
        "openai" => "OPENAI_API_KEY",
        "anthropic" => "ANTHROPIC_API_KEY",
        "together_ai" => "TOGETHER_API_KEY",
        "groq" => "GROQ_API_KEY",
        "deepseek" => "DEEPSEEK_API_KEY",
        "fireworks_ai" => "FIREWORKS_AI_API_KEY",
        "mistral" => "MISTRAL_API_KEY",
        "openrouter" => "OPENROUTER_API_KEY",
        "perplexity" => "PERPLEXITY_API_KEY",
        "xai" => "XAI_API_KEY",
        "nvidia_nim" => "NVIDIA_API_KEY",
        "cerebras" => "CEREBRAS_API_KEY",
        "sambanova" => "SAMBANOVA_API_KEY",
        "deepinfra" => "DEEPINFRA_API_KEY",
        "azure" | "azure_ai" => "AZURE_API_KEY",
        // Docker Model Runner runs locally and does not require an API key.
        // Return None so no Authorization header is sent.
        "docker_model_runner" | "ollama" | "vllm" | "lm_studio" => return None,
        _ => return None,
    };

    std::env::var(env_var).ok()
}

/// Return the request timeout duration from model params, defaulting to 600 s.
pub fn request_timeout(params: &ModelParams) -> std::time::Duration {
    std::time::Duration::from_secs_f64(params.timeout.unwrap_or(600.0))
}

/// Parse upstream provider error responses into AppError.
pub fn parse_upstream_error(status: StatusCode, body: &str) -> AppError {
    if let Ok(json) = serde_json::from_str::<serde_json::Value>(body) {
        let message = json
            .get("error")
            .and_then(|e| e.get("message"))
            .and_then(|m| m.as_str())
            .unwrap_or(body);
        AppError::ProviderError {
            status,
            message: message.to_string(),
        }
    } else {
        AppError::ProviderError {
            status,
            message: body.to_string(),
        }
    }
}
