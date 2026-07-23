# GitHub Code Review Agent

An automated, multi-agent code review service that reviews pull requests, repositories, and code snippets — then posts structured feedback (summary + inline comments) back to GitHub.

---

## Agenda

Ship continuous, multi-dimensional code review without waiting on a human first pass.

| Goal | Outcome |
|------|---------|
| **Automate PR review** | React to GitHub webhooks and review PRs as they open or update |
| **Cover more than style** | Security, quality, performance, style, and testing in parallel |
| **Actionable feedback** | Scored findings with severity, file/line hints, and fix suggestions |
| **Scale large diffs** | Chunk large changes and merge specialist reports into one review |
| **Optional depth** | Enrich security with static analysis (gosec, semgrep) when available |

---

## What It Does

The service runs **five specialist LLM agents** on each review, then an **aggregator** merges and prioritizes findings:

```
GitHub Webhook / REST API
        │
        ▼
   Review Service
        │
        ▼
 Multi-Agent Orchestrator
        │
   ┌────┴────┬─────────┬──────────┬────────┐
   ▼         ▼         ▼          ▼        ▼
Security  Quality  Performance  Style    Test
(+ gosec/semgrep)
        │
        ▼
   Aggregator → scored review
        │
        ▼
 GitHub PR comment + inline notes (optional)
```

**Review modes:**

| Mode | Trigger | What happens |
|------|---------|--------------|
| **Snippet** | `POST /api/v1/review` | Review pasted code |
| **Pull request** | `POST /api/v1/review/pr` or GitHub webhook | Fetch PR patches, review, optionally post comments |
| **Repository** | `POST /api/v1/review/repo` | Review selected repo files (sync or async) |
| **Job status** | `GET /api/v1/review/jobs/:id` | Poll async repo/PR jobs (Redis required) |

On PRs, the agent posts a **neutral COMMENT** review (does not approve or request changes): markdown summary plus up to **20 inline comments**, preferring critical/high severity.

---

## What You Can Achieve

- **Faster first-pass reviews** — every PR gets security, quality, performance, style, and test feedback automatically
- **Consistent standards** — same checklist on every change, across languages
- **Fewer missed security issues** — LLM review plus optional gosec/semgrep findings
- **Clear priorities** — findings ranked by severity (`critical` → `info`) with scores for overall, maintainability, readability, security, and performance
- **CI/CD integration** — webhook on `opened` / `synchronize` / `reopened`, or call the REST API from pipelines
- **Async heavy work** — Redis + Asynq for long repo/PR jobs without blocking the HTTP request

---

## Checks Covered

### 1. Security Agent

| Check area | Examples |
|------------|----------|
| OWASP Top 10 | Injection, XSS, CSRF, SSRF, insecure deserialization |
| Secrets | Hardcoded API keys, passwords, tokens |
| Auth | Authentication / authorization flaws |
| Crypto | Weak or insecure cryptography |
| Static tools | **gosec** (Go), **semgrep** (multi-language) when installed |

Tool findings are treated as factual, prepended to the report, and can cap the security score when issues are found.

### 2. Quality / Maintainability Agent

- Complexity and readability  
- Error handling and edge cases  
- Coupling, cohesion, naming  
- Function length and long-term maintainability  

### 3. Performance Agent

- Inefficient algorithms and hot loops  
- Unnecessary allocations  
- N+1 queries and blocking I/O  
- Missing caching and scalability concerns  

### 4. Style Agent

- Formatting consistency  
- Idiomatic language patterns  
- Documentation quality  
- Naming conventions and best practices  

### 5. Test Agent

- Missing unit / integration tests  
- Weak assertions  
- Untested edge cases  
- Test anti-patterns  

### Aggregator

- Deduplicates overlapping findings (keeps highest severity)  
- Builds a unified quality assessment  
- Prioritizes and caps output at **20 findings**  

**Finding severities:** `critical` · `high` · `medium` · `low` · `info`

---

## Supported Languages

Go, Python, JavaScript/TypeScript/TSX, Java, Rust, Ruby, PHP, C#, YAML, JSON, SQL, and Markdown. Common noise paths (vendor, `node_modules`, binaries, lockfiles) are skipped.

---

## Quick Start

```bash
# 1. Configure
cp .env.example .env
# Edit .env — set LLM_API_KEY and GITHUB_TOKEN (and webhook secret if used)

# 2. Run with Docker (app + Redis)
docker compose up --build

# 3. Health check
curl http://localhost:8080/health
```

Or with Make:

```bash
make build   # build images
make up      # start services
make logs    # follow logs
make down    # stop
```

### Required configuration

| Variable | Purpose |
|----------|---------|
| `LLM_API_KEY` | Groq / OpenAI-compatible API key |
| `GITHUB_TOKEN` | Fetch PRs/repos and post review comments |
| `GITHUB_WEBHOOK_SECRET` | Verify GitHub webhook signatures |
| `REDIS_ADDR` | Enable async queue (set by Compose to `redis:6379`) |

See `.env.example` for the full list (LLM provider/model, comment posting, chunk/file limits, gosec/semgrep paths).

---

## API Overview

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness |
| `POST` | `/api/v1/review` | Review code snippet |
| `POST` | `/api/v1/review/pr` | Review a pull request |
| `POST` | `/api/v1/review/repo` | Review a repository |
| `GET` | `/api/v1/review/jobs/:id` | Async job status |
| `POST` | `/api/v1/webhooks/github` | GitHub webhook receiver |

**Example — review a PR:**

```bash
curl -X POST http://localhost:8080/api/v1/review/pr \
  -H "Content-Type: application/json" \
  -d '{"owner":"ORG","repo":"REPO","pr":123}'
```

**Example — review a snippet:**

```bash
curl -X POST http://localhost:8080/api/v1/review \
  -H "Content-Type: application/json" \
  -d '{"code":"func main() {}","language":"go","file_path":"main.go"}'
```

### GitHub webhook setup

1. Point the webhook URL to `https://<host>/api/v1/webhooks/github`
2. Content type: `application/json`
3. Secret: same value as `GITHUB_WEBHOOK_SECRET`
4. Events: **Pull requests** (handles `opened`, `synchronize`, `reopened`)

---

## Architecture (high level)

| Layer | Role |
|-------|------|
| `cmd/server` | Process entry and wiring |
| `handler` | HTTP + webhook HMAC |
| `service` | Review flows and PR comment posting |
| `reviewer` | Multi-agent reviewer facade |
| `orchestrator` | Parallel agents + chunk merge |
| `agents` | Security, Quality, Performance, Style, Test + Aggregator |
| `tools` | Optional gosec / semgrep |
| `github` | PR/repo fetch, diff line mapping, inline reviews |
| `llm` | OpenAI-compatible chat client |
| `queue` | Redis / Asynq background jobs |
| `chunker` | Size-based batching and language detection |

**Stack:** Go · Gin · zap · go-github · Asynq/Redis · Groq or OpenAI-compatible LLM · Docker Compose

---

## Project Status Notes

- Production path uses the **multi-agent reviewer** (not the legacy single-LLM reviewer).
- Static analysis tools are **optional**; they are skipped if not on `PATH` (not bundled in the default Docker image).
- Without Redis, webhooks still work via an in-process background job and return `202`.

---

## License

Add your preferred license here.
