use std::collections::HashMap;
use std::env;
use std::path::Path;

use serde::Deserialize;

#[derive(Debug, Clone, Deserialize)]
pub struct Config {
    #[serde(default)]
    pub model_list: Vec<ModelEntry>,
    #[serde(default)]
    pub general_settings: GeneralSettings,
    #[serde(default)]
    pub gateway_settings: GatewaySettings,
    #[serde(default)]
    pub environment_variables: HashMap<String, String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ModelEntry {
    pub model_name: String,
    pub params: ModelParams,
    #[serde(default)]
    #[allow(dead_code)]
    pub tpm: Option<u64>,
    #[serde(default)]
    #[allow(dead_code)]
    pub rpm: Option<u64>,
    #[serde(default)]
    #[allow(dead_code)]
    pub max_parallel_requests: Option<u64>,
    #[serde(default)]
    #[allow(dead_code)]
    pub timeout: Option<f64>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ModelParams {
    pub model: String,
    #[serde(default)]
    pub api_key: Option<String>,
    #[serde(default)]
    pub api_base: Option<String>,
    #[serde(default)]
    pub api_version: Option<String>,
    #[serde(default)]
    pub timeout: Option<f64>,
    #[serde(default)]
    #[allow(dead_code)]
    pub max_retries: Option<u32>,
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct GeneralSettings {
    #[serde(default)]
    pub master_key: Option<String>,
    #[serde(default)]
    pub num_retries: Option<u32>,
    #[serde(default)]
    #[allow(dead_code)]
    pub request_timeout: Option<f64>,
    #[serde(default)]
    pub fallbacks: Option<Vec<HashMap<String, Vec<String>>>>,
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct GatewaySettings {
    #[serde(default)]
    #[allow(dead_code)]
    pub drop_params: Option<bool>,
    #[serde(default)]
    pub num_retries: Option<u32>,
    #[serde(default)]
    #[allow(dead_code)]
    pub request_timeout: Option<f64>,
}

/// Resolve environment variable references in a string value.
/// Supports `os.environ/VAR_NAME` and `${VAR_NAME}` syntax.
fn resolve_env_value(val: &str) -> String {
    if let Some(var_name) = val.strip_prefix("os.environ/") {
        env::var(var_name).unwrap_or_else(|_| val.to_string())
    } else if val.starts_with("${") && val.ends_with('}') {
        let var_name = &val[2..val.len() - 1];
        env::var(var_name).unwrap_or_else(|_| val.to_string())
    } else {
        val.to_string()
    }
}

impl Config {
    pub fn load(path: &Path) -> Result<Self, Box<dyn std::error::Error>> {
        let contents = std::fs::read_to_string(path)?;
        let mut config: Config = serde_yaml::from_str(&contents)?;

        // Set environment variables from config
        for (key, value) in &config.environment_variables {
            let resolved = resolve_env_value(value);
            env::set_var(key, &resolved);
        }

        // Resolve environment variable references in model params
        for entry in &mut config.model_list {
            if let Some(ref key) = entry.params.api_key {
                entry.params.api_key = Some(resolve_env_value(key));
            }
            if let Some(ref base) = entry.params.api_base {
                entry.params.api_base = Some(resolve_env_value(base));
            }
            if let Some(ref version) = entry.params.api_version {
                entry.params.api_version = Some(resolve_env_value(version));
            }
        }

        // Resolve master_key
        if let Some(ref key) = config.general_settings.master_key {
            config.general_settings.master_key = Some(resolve_env_value(key));
        }

        Ok(config)
    }
}

/// Determine the LLM provider from the model string.
/// Format: "provider/model_name" or just "model_name" (defaults to openai).
pub fn parse_provider_model(model: &str) -> (&str, &str) {
    if let Some(idx) = model.find('/') {
        let provider = &model[..idx];
        let model_name = &model[idx + 1..];
        (provider, model_name)
    } else {
        ("openai", model)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_provider_model() {
        assert_eq!(parse_provider_model("openai/gpt-4o"), ("openai", "gpt-4o"));
        assert_eq!(
            parse_provider_model("anthropic/claude-3-opus-20240229"),
            ("anthropic", "claude-3-opus-20240229")
        );
        assert_eq!(
            parse_provider_model("gpt-3.5-turbo"),
            ("openai", "gpt-3.5-turbo")
        );
        assert_eq!(
            parse_provider_model("docker_model_runner/ai/smollm2"),
            ("docker_model_runner", "ai/smollm2")
        );
    }

    #[test]
    fn test_resolve_env_value_plain() {
        assert_eq!(resolve_env_value("plain-value"), "plain-value");
    }
}
