use futures::StreamExt;
use reqwest::Client;

use crate::config::ModelParams;
use crate::error::AppError;
use crate::types::{
    ChatCompletionRequest, ChatCompletionResponse, EmbeddingRequest, EmbeddingResponse,
};

use super::{build_api_url, request_timeout, resolve_api_key, send_and_check, ByteStream, Provider};

/// OpenAI-compatible provider.
///
/// Works for OpenAI and any provider that speaks the OpenAI REST API format:
/// Together AI, Groq, Mistral, DeepSeek, Fireworks, OpenRouter, vLLM, Ollama,
/// Docker Model Runner, and more.
pub struct OpenAIProvider {
    client: Client,
}

impl OpenAIProvider {
    pub fn new() -> Self {
        Self {
            client: Client::new(),
        }
    }
}

#[async_trait::async_trait]
impl Provider for OpenAIProvider {
    async fn chat_completion(
        &self,
        request: &ChatCompletionRequest,
        params: &ModelParams,
    ) -> Result<ChatCompletionResponse, AppError> {
        let (provider_name, actual_model) = crate::config::parse_provider_model(&params.model);
        let url = build_api_url(provider_name, params, "/chat/completions");

        let mut outgoing = request.clone();
        outgoing.model = actual_model.to_string();
        outgoing.stream = Some(false);

        let mut req = self.client.post(&url).json(&outgoing);
        if let Some(api_key) = resolve_api_key(provider_name, params) {
            req = req.bearer_auth(&api_key);
        }
        req = apply_azure_version(req, provider_name, params);
        req = req.timeout(request_timeout(params));

        let response = send_and_check(req, "provider").await?;
        let resp: ChatCompletionResponse = response.json().await.map_err(|e| {
            AppError::Internal(format!("Failed to parse provider response: {}", e))
        })?;
        Ok(resp)
    }

    async fn chat_completion_stream(
        &self,
        request: &ChatCompletionRequest,
        params: &ModelParams,
    ) -> Result<ByteStream, AppError> {
        let (provider_name, actual_model) = crate::config::parse_provider_model(&params.model);
        let url = build_api_url(provider_name, params, "/chat/completions");

        let mut outgoing = request.clone();
        outgoing.model = actual_model.to_string();
        outgoing.stream = Some(true);

        let mut req = self.client.post(&url).json(&outgoing);
        if let Some(api_key) = resolve_api_key(provider_name, params) {
            req = req.bearer_auth(&api_key);
        }
        req = apply_azure_version(req, provider_name, params);
        req = req.timeout(request_timeout(params));

        let response = send_and_check(req, "provider").await?;
        let stream = response.bytes_stream().map(|chunk| {
            chunk.map_err(|e| AppError::Internal(format!("Stream error: {}", e)))
        });
        Ok(Box::pin(stream))
    }

    async fn embedding(
        &self,
        request: &EmbeddingRequest,
        params: &ModelParams,
    ) -> Result<EmbeddingResponse, AppError> {
        let (provider_name, actual_model) = crate::config::parse_provider_model(&params.model);
        let url = build_api_url(provider_name, params, "/embeddings");

        let mut outgoing = request.clone();
        outgoing.model = actual_model.to_string();

        let mut req = self.client.post(&url).json(&outgoing);
        if let Some(api_key) = resolve_api_key(provider_name, params) {
            req = req.bearer_auth(&api_key);
        }
        req = req.timeout(request_timeout(params));

        let response = send_and_check(req, "provider").await?;
        let resp: EmbeddingResponse = response.json().await.map_err(|e| {
            AppError::Internal(format!("Failed to parse provider response: {}", e))
        })?;
        Ok(resp)
    }
}

/// Append the Azure `api-version` query parameter when the provider is Azure.
fn apply_azure_version(
    mut req: reqwest::RequestBuilder,
    provider_name: &str,
    params: &ModelParams,
) -> reqwest::RequestBuilder {
    if provider_name == "azure" || provider_name == "azure_ai" {
        if let Some(ref version) = params.api_version {
            req = req.query(&[("api-version", version.as_str())]);
        }
    }
    req
}
