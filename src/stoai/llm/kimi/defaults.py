DEFAULTS = {
    "api_compat": "openai",
    "base_url": "https://api.moonshot.ai/v1",
    # Moonshot uses the same endpoint for both OpenAI and Anthropic compat.
    # Override in config.json if they add a separate Anthropic-compatible URL.
    "base_url_anthropic": "https://api.moonshot.ai/v1",
    "api_key_env": "KIMI_API_KEY",
    "model": "kimi-k2.5",
    "sub_agent_model": "kimi-k2.5",
    "inline_model": "kimi-k2.5",
    "web_search_provider": "kimi",
    "vision_provider": "kimi",
    "thinking_model": "default",
    "thinking_sub_agent": "default",
    "rate_limit_interval": 0,
}
