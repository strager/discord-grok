# Debugging prompts

## Logging production prompts

Set `prompt_log_dir` in `config.toml` to enable logging:

```toml
prompt_log_dir = "logs"
```

Each bot response writes a JSON file to that directory containing:

- Structured data (context messages, trigger message ID)
- The full prompt sent to Grok (system + user content)
- The response from Grok

Files are named `<timestamp>_<message_id>.json` and are pretty-printed for readability.

## Replaying prompts

After making prompt changes, replay a logged interaction to see what changed:

```sh
./discord-grok replay -no-call -full-context logs/2026-03-07T13-45-02-05-00_123456789.json
```

This reconstructs the prompt from the structured data using the current code and diffs it against the original.

### Flags

| Flag | Description |
|------|-------------|
| `-config` | Path to config file (default: `config.toml`) |
| `-no-call` | Skip calling the Grok API; only diff prompts |
| `-full-context` | Show full content for identical sections and full context for diffs |

### With API call

To also compare Grok's response, omit `-no-call`:

```sh
./discord-grok replay logs/2026-03-07T13-45-02-05-00_123456789.json
```

This requires `xai_api_key` in the config file. The output shows three diffs:

1. **System Prompt** - changes to the system prompt text
2. **User Content** - changes to how messages are formatted (XML structure, mention replacement, etc.)
3. **Response** - differences in Grok's output (inherently nondeterministic)
