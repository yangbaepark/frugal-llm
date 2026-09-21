# Frugal LLM (Golang)

> **No single model excels at every workload — routing prompts to specialized engines maximizes accuracy while protecting your budget.**

**Frugal LLM** is a lightweight, high-performance LLM proxy server written in Go. It provides an **OpenAI-compatible unified API interface** (`/v1/chat/completions`) that dynamically auto-selects the optimal specialized provider and model architecture for every incoming prompt to maximize response quality while remaining frugal with your API budget.

---

## Why Frugal LLM? Domain-Aware Workload Matching

Different tasks require distinct model capabilities, parameters, and architectural strengths (e.g., Mixture-of-Experts, high-speed Flash inference, or reasoning-tuned models). Using a massive frontier model for every simple prompt burns budget unnecessarily, while sending complex reasoning or legal analysis to lightweight models degrades accuracy.

**Frugal LLM** automatically evaluates incoming prompts via an exclusive, template-driven LLM classifier engine using the cheapest possible model to match each workload to its ideal target model architecture:

| Workload Domain | Task Examples | Target Model Architecture / Class | Budget & Capability Impact |
| :--- | :--- | :--- | :--- |
| **Casual & Conversational** | Greetings, small talk, basic Q&A, text cleanup | **Flash & High-Speed Engines** | **Ultra Low Cost & Latency** |
| **Code & Development** | Syntax fixes, routine scripts, CSS/HTML formatting | **Code-Optimized Models** | **Balanced Development Cost** |
| **Math & Reasoning** | Calculus, algebra, proofs, scientific calculations | **Reasoning & MoE (Mixture of Experts) Models** | **Deep Analytical Precision** |
| **Legal & Architecture** | Contract analysis, compliance, concurrency debugging | **Frontier & Flagship Models** | **Maximum Accuracy & Context** |

---

## Supported Providers

- **OpenAI**
- **Google Gemini**
- **Anthropic Claude**
- **DeepSeek** (OpenAI Protocol)
- **Groq** (OpenAI Protocol)
- **Together AI** (OpenAI Protocol)
- **Generic Local Server** (LM Studio, Ollama, llama.cpp, vLLM, Jan.ai)
- **HuggingFace**

---

## Key Features

- **Direct Provider & Model Classification:** Exclusive prompt-based routing via a template-driven classifier engine using the cheapest possible model (e.g. local LM Studio/Ollama or ultra-low-cost Flash models). The classifier directly inspects active providers and their model use-case descriptions (`templates/classifier_prompt.tmpl`) to route prompts directly to target `provider/model` endpoints without manual category configuration.
- **System One / Jev Decision Engine Support (Experimental):** Support for non-autoregressive, calibrated decision engines ([TypeSafe Jev](https://www.datacamp.com/blog/system-one-models-jev), [Kev](https://github.com/jaredpalmer/kev), [Open-Jev-9B](https://huggingface.co/ZefanCai/Open-Jev-9B)) for single-forward-pass prompt classification with ~50-100ms latency.
- **Pluggable Prioritized Classifier Pipeline:** Sequentially chains fast decision engines (System One) with fallback LLM judges and default safety models, ensuring zero routing failures.
- **Unified OpenAI Specification:** Zero code refactoring — compatible with standard OpenAI SDKs, LangChain, LlamaIndex, Cline, Cursor, or `curl`.
- **Flexible Payload Unmarshaling:** Native support for both string and structured array content parts sent by coding assistants (Cline, Cursor).
- **Environment Variable Activation:** Remote providers activate when their API key is set; local servers activate when `FRUGAL_LLM_LOCAL_BASE_URL` is set.
- **SSE Streaming Support:** Full real-time Server-Sent Events (SSE) streaming for all supported providers.
- **Provider Retries & Exponential Backoff:** Built-in per-provider retry logic with randomized jitter and configurable timeout caps.
- **Low Latency & High Concurrency:** Powered by Go's native HTTP router and lightweight non-blocking goroutines.

---

## Quick Start

### 1. Basic Setup (Zero-Edit Configuration)

Basic users do **not need to edit any configuration files**. Simply export the environment variables for the LLM providers you want to use. Frugal LLM automatically activates providers matching your exported keys at startup:

```bash
# Cloud Provider API Keys (Exporting automatically enables the provider)
export FRUGAL_LLM_OPENAI_API_KEY="sk-..."
export FRUGAL_LLM_WORK_OPENAI_API_KEY="sk-..."
export FRUGAL_LLM_ANTHROPIC_API_KEY="sk-ant-..."
export FRUGAL_LLM_GEMINI_API_KEY="AIzaSy..."
export FRUGAL_LLM_DEEPSEEK_API_KEY="sk-..."
export FRUGAL_LLM_GROQ_API_KEY="gsk_..."
export FRUGAL_LLM_TOGETHER_API_KEY="..."
export FRUGAL_LLM_TYPESAFE_API_KEY="ts-..." # Optional: Enables ultra-fast System One / Jev routing (Experimental)

# Local LLM Server Endpoint (LM Studio, Ollama, llama.cpp, vLLM, Jan.ai)
# e.g., "http://localhost:1234/v1" for LM Studio, "http://localhost:11434/v1" for Ollama
export FRUGAL_LLM_LOCAL_BASE_URL="http://localhost:1234/v1"
```

### 2. Advanced Setup (Custom YAML Configuration)

Advanced users can copy the default `config.yaml` to customize endpoints, model lists, cost factors (`$0`, `$`, `$$`, `$$$`, `$$$$`), or retry limits:

```bash
cp config.yaml my_custom_config.yaml
export FRUGAL_LLM_CONFIG="my_custom_config.yaml"
```

In `config.yaml`, providers resolve settings directly from environment variables:

```yaml
providers:
  - name: "openai"
    type: "openai"
    base_url: "https://api.openai.com/v1"
    api_key: "${FRUGAL_LLM_OPENAI_API_KEY}"
    models:
      - name: "gpt-4o"
        cost_factor: "$$"
        description: "Flagship model balanced across reasoning, vision, and coding."

  # Generic Local LLM Server (LM Studio, Ollama, llama.cpp, vLLM, Jan.ai)
  - name: "local"
    type: "openai"
    base_url: "${FRUGAL_LLM_LOCAL_BASE_URL}"
    api_key: "${FRUGAL_LLM_LOCAL_API_KEY}"
    models:
      - name: "local-model"
        cost_factor: "$0"
        description: "Locally hosted open-weights model for zero-cost routing & offline tasks."
```

### 3. Run the Server

```bash
go run cmd/proxy-server/main.go
```

Or build a compiled single binary:

```bash
go build -o frugal-llm cmd/proxy-server/main.go
./frugal-llm
```

---

## Usage Examples

### 1. Auto-Selected Workload Routing Request

Use `"model": "auto"` (or `"frugal-router"`) to let Frugal LLM auto-select the best specialized model for the workload:

```bash
# Automatically routed to Reasoning / MoE model tier for Math
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "auto",
    "messages": [{"role": "user", "content": "Solve the differential equation dy/dx + 2y = e^(-x)."}]
  }'
```

```bash
# Automatically routed to Frontier / Flagship model tier for Legal
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "auto",
    "messages": [{"role": "user", "content": "What are the legal liability implications in this contract?"}]
  }'
```

### 2. Explicit Provider & Model Requests

```bash
# Direct provider routing
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai/gpt-4o",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

```bash
# Direct provider streaming
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic/claude-sonnet-5",
    "messages": [{"role": "user", "content": "Explain quantum computing in one sentence."}],
    "stream": true
  }'
```

---

## System One / Jev Decision Classifier (Experimental)

> [!WARNING]
> **Experimental Feature:** System One / Jev decision classifier support is currently in experimental preview. While non-autoregressive decision models offer ultra-low latency prompt routing (~50–100ms), external API formats and open-weights model backends may evolve.

### What is a System One Decision Engine?

Traditional LLM routing judges are **autoregressive** — they generate tokens sequentially to explain and decide routing, introducing hundreds of milliseconds (or seconds) of overhead before the actual model request starts.

**System One models** (such as [TypeSafe Jev](https://www.datacamp.com/blog/system-one-models-jev), [Kev](https://github.com/jaredpalmer/kev), and [Open-Jev-9B](https://huggingface.co/ZefanCai/Open-Jev-9B)) are non-autoregressive decision engines. They evaluate the user's prompt against all active candidate models in a single forward pass, outputting calibrated classification probabilities in **~50–100ms**.

### Pluggable Cascaded Routing Pipeline

Frugal LLM executes classifiers in the exact order declared in `config.yaml`. If an upstream classifier is inactive, fails, or produces a decision below your confidence threshold, routing seamlessly falls back to subsequent classifiers:

```
Incoming Prompt ("model": "auto")
        │
        ▼
┌────────────────────────────────────────────────────────┐
│ 1. System One Classifier (TypeSafe Jev / Kev)          │
│    - Fast single forward pass (~50-100ms)              │
│    - Auto-skipped (0ms) if remote API key is unset     │
│    - Evaluates decision confidence vs threshold        │
└───────────────────────────┬────────────────────────────┘
                            │ (Skipped, low confidence, or error)
                            ▼
┌────────────────────────────────────────────────────────┐
│ 2. Standard LLM Classifier (Local / Flash Judge)       │
│    - Template-driven prompt analysis                   │
└───────────────────────────┬────────────────────────────┘
                            │ (Provider failure or timeout)
                            ▼
┌────────────────────────────────────────────────────────┐
│ 3. Fallback Model (e.g., local-model / gpt-4o)         │
│    - Guaranteed routing safety net                     │
└────────────────────────────────────────────────────────┘
```

### Configuration & Deployment Modes

#### Option A: Hosted TypeSafe API (Cloud)

To use hosted TypeSafe Jev, export your API key:

```bash
export FRUGAL_LLM_TYPESAFE_API_KEY="ts-..."
```

In `config.yaml`:
```yaml
dynamic_routing:
  classifiers:
    - name: "typesafe-jev"
      type: "system-one"
      base_url: "https://api.typesafe.ai/v1"
      api_key: "${FRUGAL_LLM_TYPESAFE_API_KEY}"
      model: "jev-latest"
      confidence_threshold: 0.70  # Min confidence (0.0–1.0) to accept choice
      timeout_ms: 3000
      max_prompt_chars: 4000
```
> **Zero-Overhead Skipping:** If `FRUGAL_LLM_TYPESAFE_API_KEY` is not exported, Frugal LLM automatically skips remote System One endpoints with 0ms overhead and routes directly through the next classifier in the pipeline.

#### Option B: Self-Hosted Open-Weights (Kev / Open-Jev)

If you run open-weights decision models locally (via vLLM, SGLang, or custom Kev runtime at `http://localhost:8009/v1`):

```yaml
dynamic_routing:
  classifiers:
    - name: "local-kev"
      type: "system-one"
      base_url: "http://localhost:8009/v1"
      model: "kev-latest"         # Options: "kev-latest", "kev-4b", "open-jev-9b"
      confidence_threshold: 0.75
      timeout_ms: 2000
      max_prompt_chars: 4000
```
> Local endpoints (`localhost`, `127.0.0.1`, `0.0.0.0`) remain active without requiring an API key.

### Configuration Parameters

| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `type` | `string` | `"system-one"` | Classifier engine type (`"system-one"` or `"llm"`). |
| `base_url` | `string` | `"https://api.typesafe.ai/v1"` | API endpoint base URL (automatically routes to `/systemone` or `/chat/completions`). |
| `model` | `string` | `"jev-latest"` | Target decision engine model name (`jev-latest`, `kev-latest`, `open-jev-9b`, etc.). |
| `confidence_threshold` | `float` | `0.70` | Minimum probability score (0.0 to 1.0) required to accept the choice. If below, falls back to the next classifier. |
| `timeout_ms` | `int` | `3000` | Maximum decision timeout before failing over to the next classifier. |
| `max_prompt_chars` | `int` | `4000` | Input prompt character cap for classification (does not truncate the prompt sent to the destination model). |

---

## Project Architecture

```
frugal-llm/
├── cmd/
│   └── proxy-server/
│       └── main.go       # Server entrypoint & graceful shutdown
├── config.yaml           # Provider definitions & dynamic routing config
├── templates/
│   └── classifier_prompt.tmpl # Go template for LLM classifier prompts
├── internal/
│   ├── adapter/          # Provider adapters (OpenAI, Anthropic, Ollama, HuggingFace)
│   ├── config/           # Strict YAML configuration loader & validation
│   ├── logger/           # Level-based logger (debug, info, warn, error)
│   ├── model/            # OpenAI-compatible request/response JSON structs
│   ├── protocol/         # OpenAI-v1 inbound protocol route handler
│   ├── router/           # Core proxy routing engine & retry handler
│   └── selector/         # Exclusive LLM Classifier dynamic model selector
├── LICENSE               # Apache License 2.0
├── Makefile              # Build, test, and cross-compilation targets
├── Dockerfile            # Production multi-stage Dockerfile
└── go.mod
```

---

## License

Frugal LLM is open-source software licensed under the [Apache License 2.0](LICENSE).
