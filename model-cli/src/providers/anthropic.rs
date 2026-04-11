use axum::http::StatusCode;
use bytes::Bytes;
use futures::StreamExt;
use reqwest::Client;
use serde::{Deserialize, Serialize};

use crate::config::ModelParams;
use crate::error::AppError;
use crate::types::{
    ChatCompletionChunk, ChatCompletionRequest, ChatCompletionResponse, ChatChoice, ChatDelta,
    ChatMessage, ChunkChoice, EmbeddingRequest, EmbeddingResponse, Usage,
};

use super::{parse_upstream_error, resolve_api_key, ByteStream, Provider};

const ANTHROPIC_API_URL: &str = "https://api.anthropic.com/v1";
const ANTHROPIC_VERSION: &str = "2023-06-01";

/// Anthropic Messages API provider.
///
/// Translates between the OpenAI chat completion format and the Anthropic
/// Messages API, including real-time SSE stream translation.
pub struct AnthropicProvider {
    client: Client,
}

impl AnthropicProvider {
    pub fn new() -> Self {
        Self {
            client: Client::new(),
        }
    }
}

// ── Anthropic-specific request/response types ──

#[derive(Debug, Serialize)]
struct AnthropicRequest {
    model: String,
    messages: Vec<AnthropicMessage>,
    max_tokens: u64,
    #[serde(skip_serializing_if = "Option::is_none")]
    system: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    temperature: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    top_p: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    stop_sequences: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    stream: Option<bool>,
}

#[derive(Debug, Serialize)]
struct AnthropicMessage {
    role: String,
    content: serde_json::Value,
}

#[derive(Debug, Deserialize)]
struct AnthropicResponse {
    id: String,
    model: String,
    content: Vec<AnthropicContent>,
    stop_reason: Option<String>,
    usage: AnthropicUsage,
}

#[derive(Debug, Deserialize)]
struct AnthropicContent {
    #[serde(rename = "type")]
    content_type: String,
    #[serde(default)]
    text: Option<String>,
}

#[derive(Debug, Deserialize)]
struct AnthropicUsage {
    input_tokens: u64,
    output_tokens: u64,
}

#[derive(Debug, Deserialize)]
struct AnthropicStreamEvent {
    #[serde(rename = "type")]
    event_type: String,
    #[serde(default)]
    #[allow(dead_code)]
    index: Option<u32>,
    #[serde(default)]
    delta: Option<AnthropicDelta>,
    #[serde(default)]
    message: Option<AnthropicStreamMessage>,
    #[serde(default)]
    #[allow(dead_code)]
    usage: Option<AnthropicUsage>,
}

#[derive(Debug, Deserialize)]
struct AnthropicDelta {
    #[serde(rename = "type", default)]
    #[allow(dead_code)]
    delta_type: Option<String>,
    #[serde(default)]
    text: Option<String>,
    #[serde(default)]
    stop_reason: Option<String>,
}

#[derive(Debug, Deserialize)]
struct AnthropicStreamMessage {
    id: String,
    #[allow(dead_code)]
    model: String,
}

/// Convert OpenAI messages to Anthropic format.
///
/// Returns (system_prompt, user/assistant messages).
fn convert_messages(messages: &[ChatMessage]) -> (Option<String>, Vec<AnthropicMessage>) {
    let mut system_msg = None;
    let mut anthropic_messages = Vec::new();

    for msg in messages {
        if msg.role == "system" {
            if let Some(ref content) = msg.content {
                system_msg = match content {
                    serde_json::Value::String(s) => Some(s.clone()),
                    other => Some(other.to_string()),
                };
            }
        } else {
            let role = match msg.role.as_str() {
                "assistant" => "assistant",
                _ => "user", // "user", "tool" -> "user" for Anthropic
            };

            let content = msg
                .content
                .clone()
                .unwrap_or(serde_json::Value::String(String::new()));

            anthropic_messages.push(AnthropicMessage {
                role: role.to_string(),
                content,
            });
        }
    }

    (system_msg, anthropic_messages)
}

/// Map Anthropic stop_reason to OpenAI finish_reason.
fn map_stop_reason(reason: &Option<String>) -> Option<String> {
    reason.as_ref().map(|r| {
        match r.as_str() {
            "end_turn" => "stop",
            "max_tokens" => "length",
            "stop_sequence" => "stop",
            "tool_use" => "tool_calls",
            other => other,
        }
        .to_string()
    })
}

fn get_api_url(params: &ModelParams) -> String {
    if let Some(ref base) = params.api_base {
        base.trim_end_matches('/').to_string()
    } else {
        ANTHROPIC_API_URL.to_string()
    }
}

#[async_trait::async_trait]
impl Provider for AnthropicProvider {
    async fn chat_completion(
        &self,
        request: &ChatCompletionRequest,
        params: &ModelParams,
    ) -> Result<ChatCompletionResponse, AppError> {
        let base_url = get_api_url(params);
        let url = format!("{}/messages", base_url);

        let (_, actual_model) = crate::config::parse_provider_model(&params.model);
        let (system, messages) = convert_messages(&request.messages);

        let stop_sequences = request.stop.as_ref().map(|s| match s {
            crate::types::StopSequence::Single(s) => vec![s.clone()],
            crate::types::StopSequence::Multiple(v) => v.clone(),
        });

        let anthropic_req = AnthropicRequest {
            model: actual_model.to_string(),
            messages,
            max_tokens: request.max_tokens.unwrap_or(4096),
            system,
            temperature: request.temperature,
            top_p: request.top_p,
            stop_sequences,
            stream: Some(false),
        };

        let api_key = resolve_api_key("anthropic", params)
            .ok_or_else(|| AppError::Unauthorized("Missing Anthropic API key".to_string()))?;

        let timeout_secs = params.timeout.unwrap_or(600.0);
        let response = self
            .client
            .post(&url)
            .header("x-api-key", &api_key)
            .header("anthropic-version", ANTHROPIC_VERSION)
            .header("content-type", "application/json")
            .json(&anthropic_req)
            .timeout(std::time::Duration::from_secs_f64(timeout_secs))
            .send()
            .await
            .map_err(|e| AppError::ProviderError {
                status: StatusCode::BAD_GATEWAY,
                message: format!("Failed to reach Anthropic: {}", e),
            })?;

        let status = response.status();
        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            return Err(parse_upstream_error(
                StatusCode::from_u16(status.as_u16()).unwrap_or(StatusCode::BAD_GATEWAY),
                &body,
            ));
        }

        let anthropic_resp: AnthropicResponse = response.json().await.map_err(|e| {
            AppError::Internal(format!("Failed to parse Anthropic response: {}", e))
        })?;

        // Convert to OpenAI format
        let content_text = anthropic_resp
            .content
            .iter()
            .filter(|c| c.content_type == "text")
            .filter_map(|c| c.text.as_ref())
            .cloned()
            .collect::<Vec<_>>()
            .join("");

        let created = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        Ok(ChatCompletionResponse {
            id: format!("chatcmpl-{}", anthropic_resp.id),
            object: "chat.completion".to_string(),
            created,
            model: anthropic_resp.model,
            choices: vec![ChatChoice {
                index: 0,
                message: ChatMessage {
                    role: "assistant".to_string(),
                    content: Some(serde_json::Value::String(content_text)),
                    name: None,
                    tool_calls: None,
                    tool_call_id: None,
                },
                finish_reason: map_stop_reason(&anthropic_resp.stop_reason),
            }],
            usage: Some(Usage {
                prompt_tokens: anthropic_resp.usage.input_tokens,
                completion_tokens: anthropic_resp.usage.output_tokens,
                total_tokens: anthropic_resp.usage.input_tokens + anthropic_resp.usage.output_tokens,
            }),
            system_fingerprint: None,
        })
    }

    async fn chat_completion_stream(
        &self,
        request: &ChatCompletionRequest,
        params: &ModelParams,
    ) -> Result<ByteStream, AppError> {
        let base_url = get_api_url(params);
        let url = format!("{}/messages", base_url);

        let (_, actual_model) = crate::config::parse_provider_model(&params.model);
        let (system, messages) = convert_messages(&request.messages);

        let stop_sequences = request.stop.as_ref().map(|s| match s {
            crate::types::StopSequence::Single(s) => vec![s.clone()],
            crate::types::StopSequence::Multiple(v) => v.clone(),
        });

        let anthropic_req = AnthropicRequest {
            model: actual_model.to_string(),
            messages,
            max_tokens: request.max_tokens.unwrap_or(4096),
            system,
            temperature: request.temperature,
            top_p: request.top_p,
            stop_sequences,
            stream: Some(true),
        };

        let api_key = resolve_api_key("anthropic", params)
            .ok_or_else(|| AppError::Unauthorized("Missing Anthropic API key".to_string()))?;

        let timeout_secs = params.timeout.unwrap_or(600.0);
        let response = self
            .client
            .post(&url)
            .header("x-api-key", &api_key)
            .header("anthropic-version", ANTHROPIC_VERSION)
            .header("content-type", "application/json")
            .json(&anthropic_req)
            .timeout(std::time::Duration::from_secs_f64(timeout_secs))
            .send()
            .await
            .map_err(|e| AppError::ProviderError {
                status: StatusCode::BAD_GATEWAY,
                message: format!("Failed to reach Anthropic: {}", e),
            })?;

        let status = response.status();
        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            return Err(parse_upstream_error(
                StatusCode::from_u16(status.as_u16()).unwrap_or(StatusCode::BAD_GATEWAY),
                &body,
            ));
        }

        // Translate Anthropic SSE events into OpenAI-compatible SSE events.
        let model = actual_model.to_string();
        let byte_stream = response.bytes_stream();

        let created = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        let mut buffer = String::new();
        let stream = async_stream::stream! {
            let mut stream = std::pin::pin!(byte_stream);
            let mut response_id = String::from("chatcmpl-stream");

            while let Some(chunk_result) = stream.next().await {
                let chunk = match chunk_result {
                    Ok(c) => c,
                    Err(e) => {
                        yield Err(AppError::Internal(format!("Stream error: {}", e)));
                        break;
                    }
                };

                buffer.push_str(&String::from_utf8_lossy(&chunk));

                // Process complete SSE events from the buffer
                while let Some(event_end) = buffer.find("\n\n") {
                    let event_text = buffer[..event_end].to_string();
                    buffer = buffer[event_end + 2..].to_string();

                    // Accumulate all data: lines (SSE spec allows multi-line events).
                    let event_data = event_text
                        .lines()
                        .filter_map(|line| line.strip_prefix("data: "))
                        .collect::<Vec<_>>()
                        .join("\n");

                    if event_data.is_empty() || event_data == "[DONE]" {
                        continue;
                    }

                    if let Ok(event) = serde_json::from_str::<AnthropicStreamEvent>(&event_data) {
                        match event.event_type.as_str() {
                            "message_start" => {
                                if let Some(msg) = &event.message {
                                    response_id = format!("chatcmpl-{}", msg.id);
                                }
                                let openai_chunk = ChatCompletionChunk {
                                    id: response_id.clone(),
                                    object: "chat.completion.chunk".to_string(),
                                    created,
                                    model: model.clone(),
                                    choices: vec![ChunkChoice {
                                        index: 0,
                                        delta: ChatDelta {
                                            role: Some("assistant".to_string()),
                                            content: None,
                                            tool_calls: None,
                                        },
                                        finish_reason: None,
                                    }],
                                    usage: None,
                                    system_fingerprint: None,
                                };
                                if let Ok(json) = serde_json::to_string(&openai_chunk) {
                                    yield Ok(Bytes::from(format!("data: {}\n\n", json)));
                                }
                            }
                            "content_block_delta" => {
                                if let Some(delta) = &event.delta {
                                    if let Some(text) = &delta.text {
                                        let openai_chunk = ChatCompletionChunk {
                                            id: response_id.clone(),
                                            object: "chat.completion.chunk".to_string(),
                                            created,
                                            model: model.clone(),
                                            choices: vec![ChunkChoice {
                                                index: 0,
                                                delta: ChatDelta {
                                                    role: None,
                                                    content: Some(text.clone()),
                                                    tool_calls: None,
                                                },
                                                finish_reason: None,
                                            }],
                                            usage: None,
                                            system_fingerprint: None,
                                        };
                                        if let Ok(json) = serde_json::to_string(&openai_chunk) {
                                            yield Ok(Bytes::from(format!("data: {}\n\n", json)));
                                        }
                                    }
                                }
                            }
                            "message_delta" => {
                                let finish_reason = event
                                    .delta
                                    .as_ref()
                                    .and_then(|d| d.stop_reason.as_ref())
                                    .map(|r| match r.as_str() {
                                        "end_turn" => "stop".to_string(),
                                        "max_tokens" => "length".to_string(),
                                        other => other.to_string(),
                                    });

                                let openai_chunk = ChatCompletionChunk {
                                    id: response_id.clone(),
                                    object: "chat.completion.chunk".to_string(),
                                    created,
                                    model: model.clone(),
                                    choices: vec![ChunkChoice {
                                        index: 0,
                                        delta: ChatDelta {
                                            role: None,
                                            content: None,
                                            tool_calls: None,
                                        },
                                        finish_reason,
                                    }],
                                    usage: None,
                                    system_fingerprint: None,
                                };
                                if let Ok(json) = serde_json::to_string(&openai_chunk) {
                                    yield Ok(Bytes::from(format!("data: {}\n\n", json)));
                                }
                            }
                            "message_stop" => {
                                yield Ok(Bytes::from("data: [DONE]\n\n"));
                            }
                            _ => {}
                        }
                    }
                }
            }
        };

        Ok(Box::pin(stream))
    }

    async fn embedding(
        &self,
        _request: &EmbeddingRequest,
        _params: &ModelParams,
    ) -> Result<EmbeddingResponse, AppError> {
        Err(AppError::BadRequest(
            "Anthropic does not support embeddings. Use an OpenAI-compatible provider instead."
                .to_string(),
        ))
    }
}
