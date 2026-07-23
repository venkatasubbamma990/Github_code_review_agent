# GitHub Code Review Agent

An automated, multi-agent code review service that reviews pull requests, repositories, and code snippets — then posts structured feedback (summary, inline comments, and Checks) back to GitHub.

---

## Agenda

Ship continuous, multi-dimensional code review without waiting on a human first pass.

| Goal | Outcome |
|------|---------|
| **Automate PR review** | React to GitHub webhooks and review PRs as they open or update |
| **Cover more than style** | Security, quality, performance, style, and testing in parallel |
| **Actionable feedback** | Scored findings with severity, file/line hints, and fix suggestions |
| **Scale large diffs** | Chunk large changes and merge specialist reports into one review |
| **Static analysis** | gosec + semgrep bundled in Docker and folded into the security agent |

---

## What It Does

The service runs **five specialist LLM agents** on each review, then an **aggregator** merges and prioritizes findings:

```
GitHub Webhook / REST API
        │
        ▼
   Review Service  ── Redis/Asynq (webhooks + async jobs)
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
 GitHub: PR review (inline) + Check Run / commit status
```

**Review modes:**

| Mode | Trigger | What happens |
|------|---------|--------------|
| **Snippet** | `POST /api/v1/review` | Review pasted code |
| **Pull request** | `POST /api/v1/review/pr` or GitHub webhook | Context Agent briefs specialists, then review + comments/checks |
| **Repository** | `POST /api/v1/review/repo` | Review selected repo files (sync or async) |
| **Job status** | `GET /api/v1/review/jobs/:id` | Poll async jobs (results retained 24h) |

On PRs, the agent posts a **COMMENT** review (does not approve or request changes): markdown summary plus up to **20 inline comments**, preferring critical/high severity. It also posts a **Check Run** (or commit-status fallback).

---

## Quick Start

```bash
# 1. Configure
cp .env.example .env
# Edit .env — set LLM_API_KEY and GITHUB_TOKEN
# For webhooks: set GITHUB_WEBHOOK_SECRET and REDIS_ADDR

# 2. Run with Docker (app + Redis + gosec/semgrep)
docker compose up --build

# 3. Health check
curl http://localhost:8080/health
```

Or with Make:

```bash
make build   # build images
make up      # start services
make test    # unit tests
make logs    # follow logs
make down    # stop
```

### Required configuration

| Variable | Purpose |
|----------|---------|
| `LLM_API_KEY` | Groq / OpenAI-compatible API key |
| `GITHUB_TOKEN` | Fetch PRs/repos; post reviews and statuses |
| `GITHUB_WEBHOOK_SECRET` | Verify webhook signatures (**requires Redis**) |
| `REDIS_ADDR` | Async queue for webhooks and `async` repo reviews |

Optional: `GITHUB_POST_COMMENTS`, `GITHUB_POST_CHECKS`, `GOSEC_PATH`, `SEMGREP_PATH`, `MAX_CHUNK_BYTES`, `MAX_REPO_FILES`. See `.env.example`.

---

## API Overview

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness |
| `POST` | `/api/v1/review` | Review code snippet |
| `POST` | `/api/v1/review/pr` | Review a pull request |
| `POST` | `/api/v1/review/repo` | Review a repository (`async: true` to queue) |
| `GET` | `/api/v1/review/jobs/:id` | Async job status + result |
| `POST` | `/api/v1/webhooks/github` | GitHub webhook (`202` + `job_id`) |

**Example — review a PR:**

```bash
curl -X POST http://localhost:8080/api/v1/review/pr \
  -H "Content-Type: application/json" \
  -d '{"owner":"ORG","repo":"REPO","pr":123}'
```

**Example — async repo review:**

```bash
curl -X POST http://localhost:8080/api/v1/review/repo \
  -H "Content-Type: application/json" \
  -d '{"url":"https://github.com/ORG/REPO","async":true,"max_files":20}'

# Poll
curl http://localhost:8080/api/v1/review/jobs/<job_id>
```

### GitHub webhook setup

1. URL: `https://<host>/api/v1/webhooks/github`
2. Content type: `application/json`
3. Secret: same as `GITHUB_WEBHOOK_SECRET`
4. Events: **Pull requests** (`opened`, `synchronize`, `reopened`)
5. Redis must be configured — duplicate deliveries for the same head SHA are deduplicated

---

## Checks Covered

| Agent | Focus |
|-------|--------|
| **Security** | OWASP, secrets, auth, crypto + gosec/semgrep |
| **Quality** | Complexity, errors, maintainability |
| **Performance** | Hot loops, allocations, N+1, I/O |
| **Style** | Idioms, naming, docs |
| **Test** | Coverage gaps, weak assertions |
| **Aggregator** | Dedupe, prioritize, cap at 20 findings |

**Severities:** `critical` · `high` · `medium` · `low` · `info`

**Check conclusions:** score / severity → `success` · `neutral` · `failure`

---

## Local development

```bash
make tidy
make test
make run     # needs .env; Redis for webhooks/async
```

CI runs `go vet`, `go test -race`, and `go build` on every push/PR (see `.github/workflows/ci.yml`).

---

## Architecture

| Layer | Role |
|-------|------|
| `cmd/server` | Process entry and wiring |
| `handler` | HTTP + webhook HMAC |
| `service` | Review flows, inline comments, checks |
| `reviewer` | Multi-agent facade |
| `orchestrator` | Parallel agents + chunk merge |
| `agents` | Security, Quality, Performance, Style, Test + Aggregator |
| `tools` | gosec / semgrep |
| `github` | PR/repo fetch, diff mapping, reviews, checks |
| `queue` | Redis / Asynq (24h retention, idempotent PR tasks) |
| `chunker` | Size-based batching |

**Stack:** Go · Gin · zap · go-github · Asynq/Redis · Groq or OpenAI-compatible LLM · Docker Compose

---

## Project status

- Multi-agent reviewer is the only production path
- Docker image includes **gosec** and **semgrep**
- Webhooks **require Redis** (no in-process fallback)
- PR reviews use **full file content + patches** (capped by `MAX_REPO_FILES`)
- Completed async jobs retained **24 hours** for status polling

---

## License

Add your preferred license here.
