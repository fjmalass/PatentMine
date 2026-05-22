# Solution 2: Hybrid Cache Plus Async Refresh

This design implements the workflow in `C:\Users\timot\Documents\projects\patent-tracking\solution-2-hybrid-cache-async-refresh\workflow.mmd`: public lookup requests read local graph data first and only schedule USPTO refresh work in the background.

Quick visibility:

- Mermaid source: `docs/hybrid-cache-async-refresh.mmd`
- PNG render: `docs/hybrid-cache-async-refresh.png`

## Architecture

The serving path is:

1. `citationlookup.HTTPHandler` authenticates the caller when a token is configured, resolves tenant/project identity from `project_id` or `X-Tenant-ID`, applies method and patent-length abuse limits, and propagates request tracing.
2. `citationlookup.Service.Lookup` normalizes the patent identifier, for example `8164048` to `US8164048`.
3. The service checks `patent_graph_cache` plus local `citation_edges`.
4. Fresh cache returns immediately with graph data.
5. Stale cache returns stale data and creates or coalesces a refresh job.
6. Missing cache returns a pending response and creates or coalesces a refresh job.

The refresh path is:

1. `citationlookup.Worker.WorkOnce` claims one due `citation_refresh_jobs` row.
2. The worker acquires the global USPTO limiter before importer execution.
3. `citationlookup.Runner` repeats worker execution with bounded concurrency.
4. The importer fetches metadata, backward citations, forward citations, OA citations, and optional documents outside the public request path.
5. The worker materializes patents, source-to-cited edges, source-to-citing edges, expected totals, provenance, partial flags, and page-cap metadata in SQLite.
6. On transient failures or USPTO 429 responses, the worker reschedules the job with exponential backoff or `Retry-After`; repeated transient failures can open a circuit breaker and defer new upstream calls.
7. Clients poll `/lookup/status` or retry and then receive the fresh cached graph.

## Data Model

`patent_graph_cache` stores one graph cache record per tenant/project and patent:

- `project_id`, `patent_number`, `requested_patent`
- `status`: `fresh`, `stale`, `pending`, or `failed`
- `provenance_source`, `provenance_version`
- `refreshed_at`, `fresh_until`, `updated_at`
- `partial_data`, `last_error`
- `expected_citations`, `expected_cited_by`
- `forward_page_cap`, `forward_pages_fetched`, `forward_page_cap_hit`

`citation_edges` remains the materialized local graph edge table:

- `source_patent -> target_patent` with `relation_type = cites`
- `source_patent -> target_patent` with `relation_type = cited_by`, where `target_patent` is a patent that cites the source
- `review_state`, `created_at`, `refreshed_at`, `labeled_at`

`citation_refresh_jobs` is the durable queue:

- `dedupe_key`, `project_id`, `patent_number`
- `status`: `queued`, `running`, `succeeded`, or `failed`
- `retry_count`, `next_attempt_at`, `priority`, `desired_depth`
- `trace_count` for per-patent request coalescing
- `last_error`, `created_at`, `updated_at`, `claimed_at`, `completed_at`

## API Behavior

The service returns explicit HTTP semantics through `LookupResponse.HTTPStatus`:

- Fresh: `200`, `status=fresh`, graph included, `refresh_pending=false`.
- Stale: `200`, `status=stale`, stale graph included, `refresh_pending=true`, `job_id` and `retry_after` included.
- Missing: `202`, `status=pending`, no graph required, `refresh_pending=true`, `job_id` and `retry_after` included.
- Failed with usable stale data: `200`, `status=failed`, stale graph included, `last_error` remains in cache metadata and refresh is re-enqueued.
- Failed with no local data: `202` pending on a retryable queued job, or a gateway-level error if tenant quota/auth blocks the request before lookup.

Public requests must not page through USPTO. They only read local cache and enqueue or coalesce jobs.

`GET /lookup?patent=US8164048B2` returns the lookup response. `GET /lookup/status?job_id=123` returns a queued/running/completed refresh job, and `GET /lookup/status` returns aggregate queue/cache safeguard metrics for polling clients and operators.

## Worker Algorithm

1. Claim the highest-priority due queued job.
2. Acquire the global limiter, which combines token pacing, jitter, and max in-flight constraints.
3. Run the importer for the normalized patent.
4. Normalize USPTO payloads into `domain.PatentBundle`.
5. Upsert patent metadata and citation edges.
6. Mark graph cache fresh with provenance, totals, partial-data flags, page-cap details, and `fresh_until`.
7. Mark the job succeeded.
8. On 429, timeout, or transient upstream errors, reschedule with `Retry-After` when present, otherwise exponential backoff with jitter.
9. On permanent failures or exhausted retry budget, mark the job failed without blocking public callers.

## Safeguards And Monitoring

Implemented operational metrics:

- Queue depth by status.
- Oldest queued job age and oldest graph `refresh_age`.
- Cache hit, stale hit, missing, pending, failed-response, and stale-response counters.
- Per-patent coalescing rate from `trace_count`.
- Upstream request count, 429 count, timeout count, worker success/failure counts.

Recommended next operational metrics if this is promoted to a multi-tenant HTTP deployment:

- Queue depth by tenant.
- Rate-window versions of cache hit, upstream request, timeout, and retry counters.
- Retry-budget exhaustion by tenant and patent.
- Page-cap hit rate and partial-data response rate.
- Tenant quota rejections and circuit-breaker open/half-open/closed state.

Recommended controls:

- Tenant quotas before lookup.
- Per-patent dedupe in `citation_refresh_jobs`.
- Global USPTO limiter shared by all workers.
- Bounded worker concurrency.
- Retry budget with exponential backoff and jitter.
- Circuit breaker around USPTO importer calls.
- Polling, websocket, webhook, or cache-invalidation notification after successful refresh.
