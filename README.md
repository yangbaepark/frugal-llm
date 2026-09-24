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
- **Universal Agent & IDE Compatibility:** Zero code refactoring — works seamlessly with Claude Code, Hermes Agent, OpenClaw, Cursor, Cline, Aider, LangChain, or standard OpenAI SDKs.
- **Flexible Payload Unmarshaling:** Native support for both string and structured array content parts sent by coding assistants (Cline, Cursor).
- **Environment Variable Activation:** Remote providers activate when their API key is set; local servers activate when `FRUGAL_LLM_LOCAL_BASE_URL` is set.
- **SSE Streaming Support:** Full real-time Server-Sent Events (SSE) streaming for all supported providers.
- **Provider Retries & Exponential Backoff:** Built-in per-provider retry logic with randomized jitter and configurable timeout caps.
- **Low Latency & High Concurrency:** Powered by Go's native HTTP router and lightweight non-blocking goroutines.

---

## Quick Start

The fastest way to run Frugal LLM is using the official multi-architecture Docker image (`linux/amd64`, `linux/arm64`) from **Docker Hub** or **GitHub Container Registry (GHCR)**.

### 1. Run with Docker (Zero-Edit Setup)

Simply pass your API keys directly to the container — Frugal LLM automatically enables any provider whose key is present:

```bash
docker run -d --name frugal-llm \
  -p 8080:8080 \
  -e FRUGAL_LLM_OPENAI_API_KEY="sk-..." \
  -e FRUGAL_LLM_ANTHROPIC_API_KEY="sk-ant-..." \
  -e FRUGAL_LLM_GEMINI_API_KEY="AIzaSy..." \
  -e FRUGAL_LLM_DEEPSEEK_API_KEY="sk-..." \
  -e FRUGAL_LLM_GROQ_API_KEY="gsk_..." \
  -e FRUGAL_LLM_TOGETHER_API_KEY="..." \
  -e FRUGAL_LLM_TYPESAFE_API_KEY="ts-..." \
  yangbaepark/frugal-llm:latest
```

*(Or from GHCR: `ghcr.io/yangbaepark/frugal-llm:latest`)*

### 2. Or Pass an `.env` File

```bash
docker run -d --name frugal-llm \
  -p 8080:8080 \
  --env-file .env \
  yangbaepark/frugal-llm:latest
```

---

## Agent & Tool Integrations

Point your favorite AI coding assistant or autonomous agent to `http://localhost:8080/v1` and use `"model": "auto"` (or `"frugal-router"`). Frugal LLM transparently inspects each prompt and routes it to the optimal specialized model.

### 1. Claude Code (`claude-code`)

Configure Anthropic's Claude Code CLI to route through Frugal LLM:

```bash
export ANTHROPIC_BASE_URL="http://localhost:8080"
export ANTHROPIC_API_KEY="frugal-dummy"

claude --model auto
```

---

### 2. Hermes Agent & OpenClaw

For autonomous agent runtimes like [Nous Hermes Agent](https://github.com/NousResearch), **OpenClaw**, or **Pi**:

```bash
export OPENAI_BASE_URL="http://localhost:8080/v1"
export OPENAI_API_KEY="frugal-dummy"

# Run Hermes Agent with auto-routing
hermes --model auto

# Run OpenClaw
openclaw --model auto
```

---

### 3. Cursor, Cline & Roo Code (VS Code / IDEs)

Configure your IDE extension with OpenAI-compatible settings:

| Setting | Value |
| :--- | :--- |
| **API Provider** | `OpenAI Compatible` |
| **Base URL** | `http://localhost:8080/v1` |
| **Model ID** | `auto` *(or `frugal-router`)* |
| **API Key** | `frugal-dummy` *(or your configured proxy key)* |

---

### 4. Aider (Terminal Pair Programming)

```bash
export OPENAI_API_BASE="http://localhost:8080/v1"
export OPENAI_API_KEY="frugal-dummy"

aider --model openai/auto
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

## Advanced Setup & Deployment

### 1. Custom YAML Configuration

To customize endpoints, model lists, cost factors (`$0`, `$`, `$$`, `$$$`, `$$$$`), or retry budgets:

1. Copy the default configuration file:
   ```bash
   cp config.yaml my_custom_config.yaml
   ```
2. Mount your custom configuration into the Docker container:

   - **macOS / Linux**:
     ```bash
     docker run -d --name frugal-llm \
       -p 8080:8080 \
       --env-file .env \
       -v $(pwd)/my_custom_config.yaml:/app/config.yaml \
       yangbaepark/frugal-llm:latest
     ```

   - **Windows (PowerShell)**:
     ```powershell
     docker run -d --name frugal-llm `
       -p 8080:8080 `
       --env-file .env `
       -v ${PWD}/my_custom_config.yaml:/app/config.yaml `
       yangbaepark/frugal-llm:latest
     ```

   - **Windows (Command Prompt / CMD)**:
     ```cmd
     docker run -d --name frugal-llm -p 8080:8080 --env-file .env -v %cd%/my_custom_config.yaml:/app/config.yaml yangbaepark/frugal-llm:latest
     ```

---

### 2. Docker Compose (Universal for macOS / Linux / Windows)

Create a `docker-compose.yaml` file:

```yaml
services:
  frugal-llm:
    image: yangbaepark/frugal-llm:latest # or ghcr.io/yangbaepark/frugal-llm:latest
    container_name: frugal-llm
    ports:
      - "8080:8080"
    env_file:
      - .env
    # Optional: Mount custom config
    # volumes:
    #   - ./my_custom_config.yaml:/app/config.yaml
    restart: unless-stopped
```

Start the service:
```bash
docker compose up -d
```

---

### 3. Connecting Docker to Host Local Models (Ollama / LM Studio)

If you run local models (e.g. Ollama, LM Studio, llama.cpp) on your host machine:

- **macOS & Windows (Docker Desktop / WSL2)**:
  ```bash
  docker run -d --name frugal-llm \
    -p 8080:8080 \
    -e FRUGAL_LLM_LOCAL_BASE_URL="http://host.docker.internal:11434/v1" \
    yangbaepark/frugal-llm:latest
  ```

- **Linux (Native Docker Engine)**:
  ```bash
  docker run -d --name frugal-llm \
    -p 8080:8080 \
    --add-host=host.docker.internal:host-gateway \
    -e FRUGAL_LLM_LOCAL_BASE_URL="http://host.docker.internal:11434/v1" \
    yangbaepark/frugal-llm:latest
  ```

---

### 4. Python & TypeScript SDK Integrations

You can integrate Frugal LLM into custom agents using standard OpenAI SDKs or frameworks (LangChain, AutoGen, CrewAI, LlamaIndex):

#### Python SDK
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="frugal-dummy",
)

# "auto" dynamically routes to the best model based on prompt complexity
response = client.chat.completions.create(
    model="auto",
    messages=[
        {"role": "user", "content": "Analyze and resolve the concurrency deadlock in this Go routine."}
    ],
)
print(response.choices[0].message.content)
```

#### TypeScript / Node.js SDK
```typescript
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "http://localhost:8080/v1",
  apiKey: "frugal-dummy",
});

const response = await client.chat.completions.create({
  model: "auto",
  messages: [{ role: "user", content: "Write a high-performance regex for email validation." }],
});

console.log(response.choices[0].message.content);
```

---

### 5. Running from Source (Go & Make)

#### Prerequisites
- Go 1.22+
- (Optional) `make`

#### Step 1: Export Environment Variables

- **macOS / Linux (Bash / Zsh)**:
  ```bash
  export FRUGAL_LLM_OPENAI_API_KEY="sk-..."
  export FRUGAL_LLM_ANTHROPIC_API_KEY="sk-ant-..."
  export FRUGAL_LLM_GEMINI_API_KEY="AIzaSy..."
  export FRUGAL_LLM_DEEPSEEK_API_KEY="sk-..."
  export FRUGAL_LLM_GROQ_API_KEY="gsk_..."
  export FRUGAL_LLM_TOGETHER_API_KEY="..."
  export FRUGAL_LLM_TYPESAFE_API_KEY="ts-..."
  export FRUGAL_LLM_LOCAL_BASE_URL="http://localhost:11434/v1"
  ```

- **Windows (PowerShell)**:
  ```powershell
  $env:FRUGAL_LLM_OPENAI_API_KEY="sk-..."
  $env:FRUGAL_LLM_ANTHROPIC_API_KEY="sk-ant-..."
  $env:FRUGAL_LLM_GEMINI_API_KEY="AIzaSy..."
  $env:FRUGAL_LLM_DEEPSEEK_API_KEY="sk-..."
  $env:FRUGAL_LLM_GROQ_API_KEY="gsk_..."
  $env:FRUGAL_LLM_TOGETHER_API_KEY="..."
  $env:FRUGAL_LLM_TYPESAFE_API_KEY="ts-..."
  $env:FRUGAL_LLM_LOCAL_BASE_URL="http://localhost:11434/v1"
  ```

#### Step 2: Run or Build

- **Using `make` (macOS / Linux / WSL2)**:
  ```bash
  make run           # Run server directly
  make build         # Build binary to bin/frugal-llm
  make test          # Run test suite
  make cross-compile # Compile static binaries for Linux, macOS, and Windows
  ```

- **Using standard Go CLI**:
  ```bash
  go run cmd/proxy-server/main.go
  ```

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
