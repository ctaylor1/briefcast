# External Ingest API — Client Integration Guide

A guide for an external client developer wiring their application to
briefpoint's ingest API. It covers authentication, the submit payload, curl
recipes for every endpoint, status polling, and common questions.

> This is the **operational** guide. For the full design — schema, scoring
> rationale, decision history — see [`req/external-ingest.md`](../req/external-ingest.md).

All endpoints live under `/api/ingest`. In examples below the base URL is the
local dev default `http://localhost:12314`; replace it with your deployment's
host.

---

## 1. Overview

briefpoint normally discovers content through its own RSS and podcast
pipelines. The **ingest API** lets an external system push *already-processed*
content — a transcript, a summary, metadata — into briefpoint with a single
HTTP request. briefpoint does **not** transcribe audio or run an LLM on your
behalf; you submit fully-baked records, and briefpoint normalizes, embeds,
scores, and surfaces them alongside its own items.

**Who this is for:** developers of a sibling system (e.g. *Breifcast*) that
produces podcast transcripts, book notes, or article summaries and wants them
ranked and searchable inside briefpoint.

What briefpoint does with a submission:

1. Stores the item, its source, summaries, transcript, topics, and entities.
2. Embeds it locally (Qdrant vector parity with internal items).
3. Deep-scores it locally, then multiplies the score by your client weight and
   per-source weight.
4. Promotes it if it clears the threshold; otherwise archives it (still
   searchable).

Steps 2–4 run **asynchronously** after the POST returns — see
[§5 Status polling](#5-status-polling).

For the complete design (schema additions, scoring formula, status machine,
deferred features), read [`req/external-ingest.md`](../req/external-ingest.md).

---

## 2. Authentication

Every request (except first-client bootstrap, below) authenticates with a
per-client API key sent as a **bearer token**:

```http
Authorization: Bearer sk_example_abc123DEF456ghi789
```

The plaintext key is SHA-256 hashed at rest; briefpoint never stores it
recoverably. The key is shown **once** at creation (and once again on
rotation) — store it securely. If you lose it, rotate to get a new one.

### How to get a key

There are two ways a key comes into existence:

- **First-client bootstrap.** When briefpoint has *no* ingest clients yet,
  `POST /api/ingest/clients` works **without** a token. The first client is
  created as an **admin** (it can manage other clients). See the
  [bootstrap curl example](#bootstrap-the-first-client).
- **Admin creates you via the UI.** Once at least one client exists, an
  administrator adds your client from the **`/app/ingest-clients`** view (or by
  calling `POST /api/ingest/clients` with an admin key) and hands you the key
  that's displayed once.

Key facts:

- Keys carry a `sk_` prefix purely for human recognition.
- A **disabled** or **revoked** client's key returns `403`; an unknown or
  malformed token returns `401`.
- Your client identity is always derived from the key — you never put a client
  id in a request body.

---

## 3. Submitting an item

`POST /api/ingest/items` is the one endpoint you'll call most. It is an
**upsert keyed by `external_ref`**: the first POST creates the item (`201`),
re-POSTing the same `external_ref` updates it in place (`200`).

Full payload with every field annotated:

```jsonc
{
  // REQUIRED. Your stable id for this record. Scoped to your client, so the
  // same value from a different client is a different item. Re-using it
  // updates the existing item (idempotent upsert).
  "external_ref": "breifcast:ep_2026_0517_freakonomics_578",

  // OPTIONAL. Advisory hint about which pipeline stages you've pre-done.
  // briefpoint also infers automatically (e.g. if `summaries` are present it
  // skips summarization regardless). You can omit this entirely.
  "skip_steps": ["scrape", "transcribe", "summarize", "extract"],

  // REQUIRED object. The podcast / publication / book this item belongs to.
  // briefpoint auto-creates the source row the first time it sees a new
  // (client, url) pair — you don't register sources up front.
  "source": {
    "name": "Freakonomics Radio",          // REQUIRED
    "type": "podcast",                       // OPTIONAL — falls back to your
                                             //   client's default_source_type.
                                             //   One of: rss, podcast,
                                             //   newsletter, pdf, youtube,
                                             //   forum, manual, book, article,
                                             //   paper, note
    "url": "https://freakonomics.com/feed"   // OPTIONAL — identifies the source
                                             //   for auto-create/lookup
  },

  // REQUIRED object. The content item itself.
  "item": {
    "title": "Episode 578: The Hidden Cost of Compliance",  // REQUIRED
    "url": "https://freakonomics.com/podcast/578",          // OPTIONAL
    "canonical_url": "https://freakonomics.com/podcast/578", // OPTIONAL — when
                                             //   it matches another item,
                                             //   the two are grouped into a
                                             //   story cluster (never merged)
    "published_at": "2026-05-17T09:00:00Z", // OPTIONAL, ISO-8601 UTC
    "author": "Stephen Dubner",             // OPTIONAL
    "publisher_name": "Freakonomics Radio", // OPTIONAL
    "description": "..."                     // OPTIONAL
  },

  // OPTIONAL array. Pre-processed text artifacts. On update, the previously
  // active artifacts are superseded and replaced by this set.
  "processed_artifacts": [
    {
      "kind": "transcript",                 // e.g. "transcript", "segment"
      "content": "[full transcript text…]", // OPTIONAL inline text
      "token_count": 28140                  // OPTIONAL
    },
    {
      "kind": "segment",
      "segment_start_ms": 0,                // OPTIONAL, for time-bounded chunks
      "segment_end_ms": 600000,
      "content": "[first 10 min transcript…]"
    }
  ],

  // OPTIONAL array. Your summaries. One row per `kind` (upserted by kind).
  "summaries": [
    {
      "kind": "executive",                  // REQUIRED within a summary entry
      "one_sentence": "Compliance overhead now ~12% of mid-size bank opex…",
      "executive_summary": "…",
      "why_it_matters": "…",
      "suggested_use": "customer prep, daily brief",
      "caveats": "Single-source episode.",
      "model_used": "claude-sonnet-4-6",
      "segment_start_ms": null,             // OPTIONAL — set for segment summaries
      "segment_end_ms": null,
      // Per-dimension relevance notes, keyed by scoring-dimension slug.
      // Unknown slugs are silently dropped (see FAQ).
      "relevance": {
        "fsi":   "Direct — compliance burden on banks.",
        "cloud": "Tangential — touches RegTech SaaS only.",
        "ai":    "Mentions agentic AI for KYC review."
      }
    }
  ],

  // OPTIONAL array of topic slugs. Unknown slugs are auto-created (see FAQ).
  "topics": ["banking", "regulatory_compliance", "ai"],

  // OPTIONAL array of entity mentions. `type` defaults to "other" if omitted.
  "entities": [
    {"name": "JPMorgan", "type": "company", "mention": "JPMorgan"}
  ],

  // OPTIONAL. Supply your own authoritative relevance score (0–100). When
  // present, briefpoint honors it instead of computing its own deep score.
  "score_override": { "overall_relevance": 78 }
}
```

**Required fields:** `external_ref`, `source.name`, `item.title`. (`source.type`
is required by the spec but may be omitted if your client has a
`default_source_type`.) Everything else is optional. Unknown top-level keys are
ignored, so a newer client schema won't break an older briefpoint.

`ingest_client_id` is **never** accepted in the body — it's derived from your
bearer token.

### Response

```json
{
  "item_id": "01h…",
  "external_ref": "breifcast:ep_2026_0517_freakonomics_578",
  "dedupe_key": "ext:Breifcast:ep_2026_0517_freakonomics_578",
  "source_id": "01h…",
  "source_created": true,
  "story_cluster_id": null,
  "updated": false,
  "status": "ingested",
  "final_score": null,
  "promoted": false
}
```

- `201` for a new `external_ref`, `200` for an update.
- `source_created: true` the first time a new source is auto-created — useful
  for logging "added new source" on your side.
- `status: "ingested"` means the embed + score task is queued. `final_score`
  and `promoted` are `null`/`false` here and get populated asynchronously —
  poll the GET endpoint to watch them resolve (see [§5](#5-status-polling)).

### Error responses

| HTTP | Meaning |
|------|---------|
| `200` | Update of an existing `external_ref` |
| `201` | New item created |
| `401` | Missing or invalid bearer token |
| `403` | Client is disabled, or a non-admin attempted an admin operation |
| `409` | Source URL conflicts with a soft-deleted source |
| `422` | Validation error — `{ "errors": [{ "field": "...", "message": "..." }] }` |

---

## 4. curl examples

The examples use obviously-fake keys (`sk_example_…`). Substitute your real
key and host.

### Bootstrap the first client

Works **without** a token only when no clients exist yet. The first client is
forced to admin. The `api_key` in the response is shown **once** — capture it.

```bash
curl -sS -X POST http://localhost:12314/api/ingest/clients \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Breifcast",
        "description": "Podcast transcripts + summaries",
        "weight": 1.0,
        "default_source_type": "podcast"
      }'
```

```json
{
  "id": "01h8x…",
  "name": "Breifcast",
  "description": "Podcast transcripts + summaries",
  "weight": 1.0,
  "enabled": true,
  "is_admin": true,
  "default_source_type": "podcast",
  "key_prefix": "sk_example_abc1",
  "api_key": "sk_example_abc123DEF456ghi789jkl012MNO345pqr678"
}
```

### Create an additional client (admin only)

Once a client exists, creating another requires an **admin** key. Same body as
bootstrap; the new client is non-admin by default.

```bash
curl -sS -X POST http://localhost:12314/api/ingest/clients \
  -H "Authorization: Bearer sk_example_admin000ADMIN111key222" \
  -H "Content-Type: application/json" \
  -d '{ "name": "BookNotes Importer", "default_source_type": "book" }'
```

### List clients (admin only)

```bash
curl -sS http://localhost:12314/api/ingest/clients \
  -H "Authorization: Bearer sk_example_admin000ADMIN111key222"
```

Returns each client with an `item_count`. The plaintext key is never included.

### Edit a client — weight / enabled / description (admin only)

```bash
curl -sS -X PATCH http://localhost:12314/api/ingest/clients/01h8x… \
  -H "Authorization: Bearer sk_example_admin000ADMIN111key222" \
  -H "Content-Type: application/json" \
  -d '{ "weight": 1.5, "enabled": true }'
```

`weight` is clamped to `0.0–2.0`. Omitted fields are left unchanged.

### Submit a podcast episode (transcript + summary)

```bash
curl -sS -X POST http://localhost:12314/api/ingest/items \
  -H "Authorization: Bearer sk_example_abc123DEF456ghi789" \
  -H "Content-Type: application/json" \
  -d '{
        "external_ref": "breifcast:ep_2026_0517_freakonomics_578",
        "skip_steps": ["scrape", "transcribe", "summarize", "extract"],
        "source": {
          "name": "Freakonomics Radio",
          "type": "podcast",
          "url": "https://freakonomics.com/feed"
        },
        "item": {
          "title": "Episode 578: The Hidden Cost of Compliance",
          "url": "https://freakonomics.com/podcast/578",
          "canonical_url": "https://freakonomics.com/podcast/578",
          "published_at": "2026-05-17T09:00:00Z",
          "author": "Stephen Dubner",
          "publisher_name": "Freakonomics Radio"
        },
        "processed_artifacts": [
          { "kind": "transcript", "content": "[full transcript text…]", "token_count": 28140 }
        ],
        "summaries": [
          {
            "kind": "executive",
            "one_sentence": "Compliance overhead now ~12% of mid-size bank opex.",
            "executive_summary": "…",
            "why_it_matters": "…",
            "suggested_use": "customer prep, daily brief",
            "model_used": "claude-sonnet-4-6",
            "relevance": {
              "fsi": "Direct — compliance burden on banks.",
              "cloud": "Tangential — touches RegTech SaaS only."
            }
          }
        ],
        "topics": ["banking", "regulatory_compliance", "ai"],
        "entities": [ { "name": "JPMorgan", "type": "company", "mention": "JPMorgan" } ]
      }'
```

Returns `201` with `status: "ingested"`.

### Submit a book note (`source_type=book`)

```bash
curl -sS -X POST http://localhost:12314/api/ingest/items \
  -H "Authorization: Bearer sk_example_abc123DEF456ghi789" \
  -H "Content-Type: application/json" \
  -d '{
        "external_ref": "booknotes:thinking_fast_and_slow_ch12",
        "source": {
          "name": "Thinking, Fast and Slow",
          "type": "book"
        },
        "item": {
          "title": "Ch.12 — The Science of Availability",
          "author": "Daniel Kahneman",
          "publisher_name": "Farrar, Straus and Giroux"
        },
        "summaries": [
          {
            "kind": "executive",
            "one_sentence": "Availability heuristic biases risk perception toward vivid events.",
            "executive_summary": "…",
            "why_it_matters": "Shapes how clients perceive low-frequency, high-salience risks."
          }
        ],
        "topics": ["behavioral_economics", "decision_making"]
      }'
```

### Update an existing item

Re-POST with the **same `external_ref`**. Artifacts are superseded, summaries
re-upserted, and the embed + score recomputed. Returns `200` (not `201`).

```bash
curl -sS -X POST http://localhost:12314/api/ingest/items \
  -H "Authorization: Bearer sk_example_abc123DEF456ghi789" \
  -H "Content-Type: application/json" \
  -d '{
        "external_ref": "breifcast:ep_2026_0517_freakonomics_578",
        "source": { "name": "Freakonomics Radio", "type": "podcast", "url": "https://freakonomics.com/feed" },
        "item": { "title": "Episode 578: The Hidden Cost of Compliance (corrected)" },
        "summaries": [
          { "kind": "executive", "one_sentence": "Revised: compliance overhead ~14% of opex." }
        ]
      }'
```

The response will show `"updated": true`.

### Get item status

```bash
curl -sS \
  http://localhost:12314/api/ingest/items/breifcast:ep_2026_0517_freakonomics_578 \
  -H "Authorization: Bearer sk_example_abc123DEF456ghi789"
```

```json
{
  "item_id": "01h…",
  "external_ref": "breifcast:ep_2026_0517_freakonomics_578",
  "dedupe_key": "ext:Breifcast:ep_2026_0517_freakonomics_578",
  "source_id": "01h…",
  "source_created": false,
  "story_cluster_id": null,
  "updated": false,
  "status": "promoted",
  "final_score": 82.0,
  "promoted": true
}
```

`final_score` is the post-weight score (`deep_score × client.weight ×
source.weight`); `null` until scoring completes. A `404` means the ref is
unknown or belongs to another client.

### Delete an item (soft-delete)

```bash
curl -sS -X DELETE \
  http://localhost:12314/api/ingest/items/breifcast:ep_2026_0517_freakonomics_578 \
  -H "Authorization: Bearer sk_example_abc123DEF456ghi789"
```

Returns `204`. The row is tombstoned (`deleted_at` set) but retained for audit.
You can only delete your own client's items.

### Rotate a key (admin only)

Issues a new key and invalidates the old one immediately. The new `api_key` is
shown **once**.

```bash
curl -sS -X POST \
  http://localhost:12314/api/ingest/clients/01h8x…/rotate-key \
  -H "Authorization: Bearer sk_example_admin000ADMIN111key222"
```

```json
{
  "id": "01h8x…",
  "name": "Breifcast",
  "key_prefix": "sk_example_xyz9",
  "api_key": "sk_example_xyz987WVU654tsr321QPO098nml765kji432"
}
```

### Revoke a client (admin only)

```bash
curl -sS -X DELETE http://localhost:12314/api/ingest/clients/01h8x… \
  -H "Authorization: Bearer sk_example_admin000ADMIN111key222"
```

Returns `204`. Soft-deletes the client: its key stops authenticating
immediately, but its historical items and sources remain.

---

## 5. Status polling

`POST /api/ingest/items` returns the moment the database write commits, with
`status: "ingested"`. Embedding, scoring, the promotion gate, and optional
export then run **in the background**. Poll
`GET /api/ingest/items/{external_ref}` to watch the item progress:

```text
ingested    ← POST committed; embed + score task queued
   │
   ▼
embedded    ← vector upserted to Qdrant
   │
   ▼
processed   ← scored, below the promotion threshold (archived + searchable)
   │
   ├─► promoted      ← cleared the threshold (optionally exported to Obsidian)
   │
   └─► archive_only  ← fully suppressed (weight = 0, or score floored to 0)
```

`failed` is reachable from any state if the background task errors;
`final_score` stays `null` in that case.

**Terminal states:** `processed`, `promoted`, `archive_only`, `failed`. Stop
polling once you see one of these.

**Recommended polling:** a single item's embed + score is fast (typically a few
seconds). Poll every **2–5 seconds**, and back off to ~15s if it hasn't reached
a terminal state within the first half-minute. Treat anything still at
`ingested`/`embedded` after a few minutes as stuck and surface it for
investigation. (There is no webhook callback in the MVP — polling is the only
mechanism; see `req/external-ingest.md` §9.)

---

## 6. FAQ

**What if I submit the same `external_ref` twice?**
It's an idempotent upsert. The second POST updates the existing item in place
(returns `200`, `"updated": true`): artifacts are superseded and replaced,
summaries re-upserted by kind, topics/entities replaced, and the embed + score
recomputed. You won't get a duplicate. (The `external_ref` is scoped to your
client, so the same value from a *different* client is a separate item.)

**Can I delete an item?**
Yes — `DELETE /api/ingest/items/{external_ref}` soft-deletes it (`204`). The row
is tombstoned, not hard-deleted, so it's retained for audit. Deletion is
**cross-client isolated**: you can only delete items your own client submitted;
another client's ref returns `404`.

**What about audio files?**
URL fetch / audio hosting is **deferred** (`req/external-ingest.md` §2, §9).
briefpoint will not transcribe or stream audio. In the MVP you submit the
**transcript** (as a `processed_artifact`) and a **summary** — the pre-processed
text is what gets embedded and scored. You may include an audio URL in
`item.url`, but briefpoint won't fetch it.

**How do client and source weights work?**
Each client has a `weight` and each source has a `weight`, both default `1.0`,
range `0.0–2.0`. They combine multiplicatively with briefpoint's own deep
score: `final_score = deep_score × client.weight × source.weight`. The
multiplier is applied after the score is computed and before the promotion
gate, so a weight of `0` suppresses promotion entirely. Full details and the
rationale are in [`req/external-ingest.md` §4](../req/external-ingest.md).

**What happens if I submit an unknown topic slug?**
It's **auto-created**. Any slug in `topics` that briefpoint hasn't seen becomes
a new topic and is linked to the item — no pre-registration needed.

**What happens if I submit an unknown dimension slug in the relevance map?**
It's **silently dropped**. Keys in a summary's `relevance` map must match a
configured scoring dimension slug; unknown keys are ignored (this mirrors the
scoring side, which also ignores unknown slugs). The item is still ingested
normally — only the unrecognized rationale text is discarded. Check your
briefpoint instance's configured dimensions for the valid slugs.

**Do I need to register a source first?**
No. The source row is auto-created on first submission for each
`(client, source.url)` pair. You only tune its `weight` afterward (via the
admin UI or `PATCH`); you never have to register it up front.
