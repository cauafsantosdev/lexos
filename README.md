# Lexos: Event-Driven AI Document Processing Engine

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Python](https://img.shields.io/badge/Python-3.14-3776AB?logo=python&logoColor=white)](https://python.org/)
[![Next.js](https://img.shields.io/badge/Next.js-16-black?logo=next.js)](https://nextjs.org/)
[![Redis](https://img.shields.io/badge/Redis-Broker-DC382D?logo=redis&logoColor=white)](https://redis.io/)
[![MinIO](<https://img.shields.io/badge/MinIO-S3%20Storage-C72E29>)](https://min.io/)
[![llama.cpp](<https://img.shields.io/badge/llama.cpp-Qwen3%20GGUF-4EAA25>)](https://github.com/ggml-org/llama.cpp)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**[Live Application: lexos.cauafsantos.dev](https://lexos.cauafsantos.dev)**

**Lexos** is a cloud-native, event-driven AI document processing engine built around self-hosted language models. It combines a high-concurrency Go gateway with asynchronous Python workers to execute document summarization, retrieval-augmented generation, and speech transcription without blocking HTTP request handling or relying on external AI APIs.

---

## Demo

![Lexos Demo](https://i.imgur.com/KkiJhCO.gif)

---

## Key Features

* **Self-Hosted and Privacy-Focused:** Document parsing, embeddings, vector retrieval, transcription, and LLM inference run within the self-hosted Docker environment. Lexos does not require external AI APIs or send user content to third-party model providers during processing.
* **Asynchronous Processing:** Clients receive an immediate `202 Accepted` response with a task identifier while CPU-intensive workloads continue in the background, preventing long-running transcription and document-processing requests from blocking HTTP connections.
* **Real-Time Answer Streaming:** Gleaner uses Server-Sent Events to stream generated answers token by token from the Python worker, through Redis Pub/Sub and the Go gateway, to the browser.
* **S3-Compatible Object Storage:** Uploaded documents, audio files, generated results, and FAISS indexes are stored in MinIO through its S3-compatible API, keeping processing services independent of shared container filesystems.
* **Multilingual Document Retrieval:** Gleaner uses multilingual embeddings, tokenizer-aware chunking, and FAISS cosine-similarity search to retrieve relevant evidence across supported languages.
* **Isolated Automated Testing:** Go handlers use dependency-injected Redis and object-storage interfaces, while Python tests mock models, storage, and network operations through Pytest and `pytest-mock`. This allows core business and pipeline logic to be tested without loading production ML models or requiring live infrastructure.

---

## System Architecture

Lexos uses a decoupled, event-driven architecture that separates the user interface, high-concurrency API gateway, infrastructure services, and CPU-bound machine learning workloads. This allows each layer to evolve and scale independently without coupling HTTP request handling to model inference.

```mermaid
flowchart LR
    User["User"]

    subgraph FrontendLayer["Frontend"]
        direction TB
        UI["Next.js 16 Web App"]
    end

    subgraph GatewayLayer["API Layer"]
        direction TB
        Go["Go + Echo Gateway"]
    end

    subgraph Infrastructure["Infrastructure Services"]
        direction TB

        Redis["Redis<br/>Queues · Task State · Pub/Sub"]
        MinIO["MinIO<br/>S3-Compatible Object Storage"]

        Redis ~~~ MinIO
    end

    subgraph WorkerLayer["AI Worker"]
        direction TB

        Consumer["Python Task Consumer"]

        Scriber["Scriber<br/>Faster-Whisper"]
        Distiller["Distiller<br/>Qwen3"]
        Gleaner["Gleaner<br/>FastEmbed · FAISS · Qwen3"]

        Consumer --> Scriber
        Consumer --> Distiller
        Consumer --> Gleaner

        Scriber ~~~ Distiller
        Distiller ~~~ Gleaner
    end

    User --> UI
    UI -->|"HTTP requests"| Go
    Go -->|"Create state and enqueue tasks"| Redis
    Redis -->|"BLPOP queues"| Consumer

    Go -->|"Upload artifacts"| MinIO
    Consumer -->|"Read inputs and store outputs"| MinIO
    Consumer -->|"Update state and publish tokens"| Redis

    Redis -.->|"Task status and SSE events"| Go
    Go -.->|"JSON responses and SSE"| UI
```

### 1. Frontend (Next.js)

* **Developer-Focused Interface:** Provides dedicated workflows for audio transcription, document summarization, and retrieval-augmented question answering.
* **Asynchronous Task Tracking:** Uses TanStack Query to poll task state without blocking the interface.
* **Real-Time QA:** Consumes Server-Sent Events from the Go gateway and renders generated answers token by token.
* **Typed API Integration:** Communicates with the backend through a centralized TypeScript API client following the gateway's fixed contracts.

### 2. API Gateway (Go + Echo)

* **Ingestion & Validation:** A lightweight Go binary handles all incoming HTTP traffic, parses multipart file uploads, and validates payloads.
* **Storage Proxy:** Streams incoming documents and audio files directly to **MinIO** through its S3-compatible API, preventing the Go container's local disk from filling up.
* **Task Dispatch:** Generates unique `task_id`s, writes initial state to a **Redis Hash**, and pushes the task metadata onto specific Redis lists (e.g., `lexos:queue:summarization`).
* **SSE Proxy:** Subscribes to Redis Pub/Sub channels to stream LLM generation tokens back to the client in real-time via Server-Sent Events.

### 3. The Broker (Redis)

* **State Management:** Acts as the single source of truth for task statuses (`queued`, `processing`, `completed`, `failed`).
* **Message Queues:** Buffers workloads so the Python worker is never overwhelmed, ensuring CPU-intensive ML tasks are processed sequentially by the worker without overwhelming the host.

### 4. AI Worker (Python + Llama.cpp)

* **Polling Engine:** An infinite loop continuously polls the Redis queues, popping tasks and downloading the required artifacts from MinIO into temporary local storage.
* **Distiller (Summarization):** Chunks large documents heuristically and feeds them through a Map-Reduce pipeline powered by **Qwen3 (0.6B)**.
* **Gleaner (RAG):** Splits documents along exact tokenizer boundaries using the model’s Rust-based Hugging Face tokenizer, generates multilingual embeddings through **FastEmbed**, persists **FAISS** indexes, and retrieves grounded context for **Qwen**-powered question answering.
* **Scriber (Audio):** Processes audio files through **Faster-Whisper** for offline, CPU-optimized transcription.

### 5. Object Storage (MinIO)

* **Input Artifacts:** Stores uploaded documents and audio files independently of the gateway and worker containers.
* **Generated Results:** Persists summaries, transcripts, task artifacts, FAISS indexes, and chunk metadata.
* **S3 Compatibility:** Keeps the storage layer portable, allowing MinIO to be replaced by a managed S3-compatible service without redesigning the processing pipeline.

---

## Resource-Aware Deployment

Lexos was designed to run on a small, CPU-only VPS while sharing resources with other applications. Its architecture and model choices intentionally prioritize predictable memory usage, controlled CPU consumption, and low operational cost.

* **Quantized Local Generation:** Qwen3 0.6B runs as a `Q4_K_M` GGUF model through llama.cpp, reducing model storage and memory requirements compared with full-precision PyTorch inference.
* **Memory-Mapped Model Loading:** llama.cpp uses memory-mapped GGUF files, enabling demand-paged loading and allowing the host operating system to reuse mapped pages when compatible processes access the same model file.
* **ONNX-Based Embeddings:** FastEmbed runs the multilingual E5 embedding model through ONNX Runtime without loading the PyTorch or Transformers inference stack.
* **Lightweight Vector Retrieval:** Per-document FAISS `IndexFlatIP` indexes provide cosine-similarity search without requiring a separate vector database service.
* **Controlled Worker Concurrency:** CPU-heavy workloads are processed sequentially by the worker, with inference threads intentionally limited to prevent a single task from monopolizing the host.
* **Externalized State and Artifacts:** Redis and MinIO remove the need for shared local volumes, making the gateway and worker independently deployable while keeping containers stateless.
* **Deployment Target:** The complete stack is suitable for a VPS-class environment with approximately 4 virtual CPUs and 8 GB of RAM, while the AI worker is designed around an approximate 2 GB memory budget.

---

## Tech Stack

* **Gateway:** Go (Golang), Echo Framework
* **Worker:** Python 3.14
* **UI:** Next.JS 16, Tailwind CSS
* **Infrastructure:** Docker Compose, Redis, MinIO
* **Machine Learning:** Llama.cpp (Qwen3), Faster-Whisper, FastEmbed, FAISS
* **Testing:** Testify (Go), Pytest / Pytest-Mock (Python)

---

## Project Structure

```text
lexos/
├── lexos-gateway/        # Go API Gateway
│   ├── cmd/server/       # Main entry point
│   └── internal/
│       ├── handlers/     # HTTP route controllers and Dependency Injection
│       ├── mocks/        # testify/mock auto-generated interfaces
│       ├── queue/        # Redis connection and logic
│       ├── routes/       # Endpoint declaration
│       └── storage/      # MinIO streaming and retrieval
├── worker/               # Python ML Worker
│   ├── src/
│   │   ├── broker/       # Redis consumer loop and task router
│   │   ├── services/     # Distiller, Gleaner, and Scriber ML pipelines
│   │   ├── utils/        # File parsing, prompting, and singleton model loaders
│   │   └── main.py
│   └── tests/            # Isolated Pytest suite for data pipelines
├── frontend/             # Next.js web interface
│   ├── app/              # App Router pages
│   ├── components/       # Neo-brutalist UI components
│   └── lib/              # Typed Go API client
├── docker-compose.yml    # Full infrastructure orchestration
└── .github/workflows/    # CI pipelines for Go and Python tests
```

---

## How to Run

Prerequisites: **Docker** and **Docker Compose** installed.

### 1. Clone & Boot

```bash
git clone https://github.com/cauafsantosdev/lexos
cd lexos
docker-compose up --build -d
```

*Note: The initial boot may take a few minutes as the Python container downloads and caches the GGUF, embedding, and Whisper model files.*

### 2. Test the API

Submit a document for summarization:

```bash
curl -X POST http://localhost:8000/summarize \
     -H "Content-Type: application/json" \
     -d '{"document_text": "Your long text here...", "style": "executive"}'
```

Check the status using the returned `task_id`:

```bash
curl http://localhost:8000/task/<task_id>
```

---

## Engineering Decisions

### 1. Decoupling Compute from the API

Machine learning inference is heavily CPU-bound. If the API directly invoked the Python models, a burst of 5 summarization requests would lock the server, causing all subsequent traffic to time out. By inserting Redis as a broker, the Go gateway can continue accepting and validating requests while the Python worker safely drains the queue at its own pace.

### 2. Dependency Injection for Flawless Testing

To ensure the Go Gateway was fully testable in CI/CD without needing real Redis or MinIO containers, the handlers were built using strict interfaces. This allowed the injection of in-memory mocks during unit testing, ensuring business logic (payload validation, HTTP routing) could be tested in under 5 milliseconds without network I/O.

### 3. Dual Chunking Strategies

Lexos utilizes two different chunking algorithms based on the architectural need:

* **Vector Retrieval (FAISS):** Uses strict, exact-character offset slicing mapped to a Rust-based HuggingFace Tokenizer. This ensures zero data mutation and respects strict model token limits for high-fidelity cosine similarity.
* **Summarization (Map-Reduce):** Uses heuristic character-length chunking. It requires less overhead (no tokenizer instantiation) and is perfectly safe due to the generous 3,072 context window allocated to the Llama.cpp engine.

### 4. Memory Mapping for LLMs

To run a 0.6B parameter model inside a Docker container without exhausting host RAM, the Llama.cpp engine is configured to load `.gguf` files. This utilizes OS-level memory mapping, allowing the CPU to page weights directly from the disk cache, keeping the container's active memory footprint extremely lean.

---

## License

This project is licensed under the MIT License. See the `LICENSE` file for details.

---

## Contact

Cauã Santos – [LinkedIn Profile](https://www.linkedin.com/in/cauafsantosdev/) – cauafsantosdev@gmail.com

Project Link: [https://github.com/cauafsantosdev/lexos](https://github.com/cauafsantosdev/lexos)
