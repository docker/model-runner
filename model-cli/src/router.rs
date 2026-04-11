use std::collections::HashMap;
use futures::future::BoxFuture;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;

use crate::config::{parse_provider_model, Config, ModelEntry};
use crate::error::AppError;
use crate::providers::{resolve_provider, ByteStream, Provider};
use crate::types::{
    ChatCompletionRequest, ChatCompletionResponse, EmbeddingRequest, EmbeddingResponse,
    ModelListResponse, ModelObject,
};

/// A single deployment that can serve a model alias.
struct Deployment {
    entry: ModelEntry,
    provider: Box<dyn Provider>,
}

/// The gateway router manages multiple deployments per model alias and handles
/// load balancing, retries, and fallbacks.
pub struct Router {
    /// Map from model alias -> list of deployments (for load balancing).
    deployments: HashMap<String, Vec<Deployment>>,
    /// Round-robin counters per model.
    counters: HashMap<String, Arc<AtomicUsize>>,
    /// Global number of retries on failure.
    num_retries: u32,
    /// Fallback chains: model alias -> list of fallback model aliases.
    fallbacks: HashMap<String, Vec<String>>,
}

impl Router {
    /// Build a router from the parsed config.
    pub fn from_config(config: &Config) -> Result<Self, AppError> {
        let mut deployments: HashMap<String, Vec<Deployment>> = HashMap::new();

        for entry in &config.model_list {
            let (provider_name, _) = parse_provider_model(&entry.params.model);
            let provider = resolve_provider(provider_name)?;

            deployments
                .entry(entry.model_name.clone())
                .or_default()
                .push(Deployment {
                    entry: entry.clone(),
                    provider,
                });
        }

        let counters: HashMap<String, Arc<AtomicUsize>> = deployments
            .keys()
            .map(|k| (k.clone(), Arc::new(AtomicUsize::new(0))))
            .collect();

        // Parse fallbacks from config
        let mut fallbacks: HashMap<String, Vec<String>> = HashMap::new();
        if let Some(ref fb_list) = config.general_settings.fallbacks {
            for fb_map in fb_list {
                for (model, targets) in fb_map {
                    fallbacks.insert(model.clone(), targets.clone());
                }
            }
        }

        let num_retries = config
            .general_settings
            .num_retries
            .or(config.gateway_settings.num_retries)
            .unwrap_or(0);

        Ok(Self {
            deployments,
            counters,
            num_retries,
            fallbacks,
        })
    }

    /// Get the list of configured model alias names.
    pub fn model_names(&self) -> Vec<String> {
        self.deployments.keys().cloned().collect()
    }

    /// List models in OpenAI format.
    pub fn list_models(&self) -> ModelListResponse {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        let mut models: Vec<ModelObject> = self
            .deployments
            .keys()
            .map(|name| ModelObject {
                id: name.clone(),
                object: "model".to_string(),
                created: now,
                owned_by: "model-cli".to_string(),
            })
            .collect();
        models.sort_by(|a, b| a.id.cmp(&b.id));

        ModelListResponse {
            object: "list".to_string(),
            data: models,
        }
    }

    /// Pick the next deployment index for the given model using round-robin.
    fn next_index(&self, model: &str) -> Option<usize> {
        let deps = self.deployments.get(model)?;
        if deps.is_empty() {
            return None;
        }
        let counter = self.counters.get(model)?;
        Some(counter.fetch_add(1, Ordering::Relaxed) % deps.len())
    }

    /// Shared retry-and-fallback driver used by all three public request methods.
    ///
    /// `op_name` is used only in log messages (e.g. `"chat completion"`).
    /// `call` is an async closure that receives a `(&dyn Provider, &ModelParams)`
    /// pair and returns a `Result<T, AppError>`.
    ///
    /// Retry semantics: `num_retries` controls total attempts — 0 means one try,
    /// 1 means one retry (two total), etc. Each attempt picks the next deployment
    /// via round-robin, then the configured fallback chain is tried once.
    async fn run_with_retries_and_fallbacks<T, F>(
        &self,
        model: &str,
        op_name: &str,
        call: F,
    ) -> Result<T, AppError>
    where
        T: Send,
        F: for<'a> Fn(&'a dyn Provider, &'a crate::config::ModelParams) -> BoxFuture<'a, Result<T, AppError>>,
    {
        let max_attempts = (self.num_retries + 1) as usize;
        let mut last_error = None;

        let dep_count = self.deployments.get(model).map_or(0, |v| v.len());

        for attempt in 0..max_attempts {
            if let Some(idx) = self.next_index(model) {
                let dep = &self.deployments[model][idx];
                match call(dep.provider.as_ref(), &dep.entry.params).await {
                    Ok(result) => return Ok(result),
                    Err(e) => {
                        tracing::warn!(
                            "{} attempt {}/{} for model '{}' failed: {}",
                            op_name,
                            attempt + 1,
                            max_attempts,
                            model,
                            e
                        );
                        last_error = Some(e);
                    }
                }
            } else {
                break;
            }
        }

        // Try fallbacks
        if let Some(fallback_models) = self.fallbacks.get(model) {
            for fb_model in fallback_models {
                tracing::info!("Trying fallback model: {}", fb_model);
                if let Some(idx) = self.next_index(fb_model) {
                    let dep = &self.deployments[fb_model][idx];
                    match call(dep.provider.as_ref(), &dep.entry.params).await {
                        Ok(result) => return Ok(result),
                        Err(e) => {
                            tracing::warn!("Fallback '{}' failed: {}", fb_model, e);
                            last_error = Some(e);
                        }
                    }
                }
            }
        }

        if dep_count == 0 && last_error.is_none() {
            return Err(AppError::ModelNotFound(model.to_string()));
        }

        Err(last_error.unwrap_or_else(|| AppError::AllDeploymentsFailed(model.to_string())))
    }

    /// Perform a chat completion with load balancing and retries.
    pub async fn chat_completion(
        &self,
        request: &ChatCompletionRequest,
    ) -> Result<ChatCompletionResponse, AppError> {
        let request = request.clone();
        self.run_with_retries_and_fallbacks(&request.model.clone(), "chat completion", |provider, params| {
            let request = request.clone();
            let params = params.clone();
            Box::pin(async move { provider.chat_completion(&request, &params).await })
        })
        .await
    }

    /// Perform a streaming chat completion with load balancing and retries.
    pub async fn chat_completion_stream(
        &self,
        request: &ChatCompletionRequest,
    ) -> Result<ByteStream, AppError> {
        let request = request.clone();
        self.run_with_retries_and_fallbacks(&request.model.clone(), "stream", |provider, params| {
            let request = request.clone();
            let params = params.clone();
            Box::pin(async move { provider.chat_completion_stream(&request, &params).await })
        })
        .await
    }

    /// Perform an embedding request with load balancing and retries.
    pub async fn embedding(
        &self,
        request: &EmbeddingRequest,
    ) -> Result<EmbeddingResponse, AppError> {
        let request = request.clone();
        self.run_with_retries_and_fallbacks(&request.model.clone(), "embedding", |provider, params| {
            let request = request.clone();
            let params = params.clone();
            Box::pin(async move { provider.embedding(&request, &params).await })
        })
        .await
    }
}
