# Show-note link tracking: plan and status

- **Goal:** collect the links in episode show notes and resolve each one to the person, post or page it points to. Then rank what podcasts reference most, across episodes and podcasts.
- **Started:** 2026-11-05. **Status checked:** against `main` @ `8939fec` on 2026-11-08.
- This file was recovered from the implementation session. Anything marked *(added 2026-11-08)* came later.

## Status

8 PRs in total: 6 core, 1 conditional (3b) and 1 optional (7).

| # | PR | Status | Commit |
| --- | --- | --- | --- |
| 1 | Delete links with their episode or podcast; a migration removes leftover rows | Done | `27ee1fe` |
| 2 | Normalize and classify links (`internal/feedmeta/normalize.go`); no schema change | Done | `18be790` |
| 3 | `link_targets` table and `show_note_links.link_target_id`. Unique (episode, URL) index after a dedupe migration. One save path for ingest and backfill. The backfill re-processes rows from older extraction versions. | Done | `092159a` |
| 3b | Podcast Index pull for episodes that have dropped off the feed | Not started; only if the gap check shows a real shortfall | — |
| 4 | Section label for each link (`context_label`) | Done | `69252ad` |
| 5 | Ranking APIs `GET /links/top` and `GET /links/targets/:id`, boilerplate detection, supporting index | Done | `5247d8d` |
| 6 | "Most referenced" page (`/app/#/links`) and a Links tab in the episode drawer | Done | `8d30bc3` |
| 7 | Optional: follow short links; merge one person's accounts across platforms; re-extract edited show notes | Not started | — |

Related: `4c36067` makes `migrate_to_pg` copy the link tables.

**Not recorded as done:**
- **The PR 3 spot-check:** compare the top-10 links and counts for two or three charting shows with their My Podcast Data pages, by eye.
- **A browser check of the PR 6 page.** It passed unit tests, a type check, a build and a jsdom component test.
- **Running the link queries against Postgres.**

**Next:** PR 3b's read-only gap check, which decides whether 3b is worth building.

## Design choices

1. **One row per link per episode is the source of truth.** Counts are computed by query, not stored, so they can't drift after deletes, re-normalization or backfills.
2. **"Boilerplate" is decided from the data, not a hardcoded list.** A target is boilerplate for a podcast when that podcast has at least 5 episodes with links and the target appears in at least 50% of them (`db/link_stats.go`). This catches hosts, social accounts and music credits.
   - It is judged per podcast, so a host profile that is boilerplate on its own show still counts as a guest appearance elsewhere.
   - It is judged over each podcast's full history, not the date window.
   - Rankings hide boilerplate by default.
3. **Counts use the episode's publish date**, not the row's creation date. Otherwise a backfill would make every link look recent.

## How links are normalized

`feedmeta.NormalizeLink` turns a raw URL into a stable key, a clean https URL, a kind, a platform and handle, and for a post, the key of the profile that posted it.

| Raw | Key | Kind |
| --- | --- | --- |
| `x.com/Jason`, `twitter.com/jason?s=20` | `x:jason` | profile |
| `x.com/AndrewBenson/status/2115458726396310008` | `x:status:2115458726396310008`, parent `x:andrewbenson` | post |
| `tiktok.com/@allin`, `linkedin.com/company/allinpod` | per-platform keys | profile |
| `reuters.com/business/…` | `web:` plus the host and path | web |
| `rb.gy/tppkzl` | kept as is unless PR 7 resolves it | shortener |

- **Platforms recognized:** X/Twitter, Instagram, TikTok, LinkedIn, YouTube, Threads and Bluesky, plus 20 common link shorteners.
- **Facebook** links are treated as ordinary web pages for now, because their URL formats are messy.
- **X handles are lowercased**, because X treats them as case-insensitive.
- **Web pages:**
  - `www.` and trailing slashes are dropped, the host is lowercased, and http is treated as https.
  - Tracking parameters (`utm_*`, `fbclid`, `gclid` and similar) are removed and the query string is sorted.
  - The path's case is kept, because paths are case-sensitive.
- The kind is `web`, not "article", because a URL alone can't show that a page is an article.
- Test fixture: `internal/feedmeta/testdata/allin_show_notes.txt`. Its 27 links come out as 10 profiles, 8 posts each tied to its author, 1 shortener and 8 web pages.

**Versioning:** `feedmeta.LinkExtractionVersion` (currently 2) is stored on each link row. Bump it whenever a rule change would alter keys or labels. The backfill then re-processes the affected episodes. It checks the row's version, not the target's, because targets are shared across episodes.

## Schema as built

- **`link_targets`:**
  - Columns: unique `normalized_key`, `canonical_url`, `kind`, `platform`, `handle`, `domain` and `extraction_version`.
  - `parent_target_id` links a post to its author's profile.
- **`show_note_links`** gained:
  - `link_target_id` (indexed), `context_label` and `extraction_version`.
  - A unique index on (`podcast_item_id`, `url`).
  - An index on (`podcast_id`, `link_target_id`, `podcast_item_id`).
- URLs over 2,048 bytes are skipped with a logged warning. This keeps index entries under Postgres's size limit.

**Planned but not built:**
- `display_title`: the anchor text stays on each link row instead.
- Short-link resolution columns: deferred to PR 7.
- A `source` column: held for 3b.

## Section labels (PR 4)

- **What counts as a label:** a heading (`h1`–`h6`), or the last line or sentence before a link that ends in ":".
- **Carrying forward:** a label applies to later links until a new one appears.
- **Ignored:** labels over 120 characters, which are treated as prose. Skipped links, such as app-store links, are consumed whole, so their text can't become a label.

On the All-In fixture this gives "Follow on X", "Intro Music Credit", "Referenced in the show" and so on. It is a heuristic: show notes with no clear sections can carry one label over too many links.

## APIs

All routes sit behind the same password check as every other route.

| Route | What it returns |
| --- | --- |
| `GET /links/top` | Targets ranked by how many episodes link to them. Filters: `kind`, `platform`, `podcastId`, `since`, `until` (by publish date; a plain-date `until` includes that day), `includeBoilerplate` and `limit` (1–200, default 50). Bad parameters get a 400. |
| `GET /links/targets/:id` | One target, its parent profile, episode and podcast totals, a per-podcast breakdown with boilerplate flags, and the newest episodes that link to it (`limit` 1–500, default 100). An unknown ID gets a 404. |
| `GET /podcastitems/:id/links` | The episode's links, including `LinkTargetID`, `ContextLabel` and `ExtractionVersion`. |
| `POST /settings/backfill-links` | Starts the backfill: 202, or 409 if it's already running. |
| `GET /settings/backfill-links` | `{"running": true \| false}` |

An episode counts once per target, even when it links to the same target twice (for example `x.com/jason` and `twitter.com/jason`).

## Operating notes

- **Run the backfill after deploying** any change that bumps `LinkExtractionVersion`. Until it runs, older episodes are missing from the rankings.
- **Back up before the first start of a build that adds the PR 1 or PR 3 migrations.** They delete orphaned and duplicate link rows. The backup is the rollback.
- *(added 2026-11-08, fixed 2026-11-09)* **On SQLite, the backfill used to delay transcription.** It saved one episode per transaction in a tight loop, so the per-minute transcription job failed with `database is locked` for the whole run (about 8 minutes for about 5,000 episodes). It now pauses 250 ms after every 20 episodes. That has not yet been measured on a production-sized copy.

## Known limitations

- The page shows the top 50, with no way to load more.
- There is no backfill button; the backfill is started through the API.
- A person's posts don't count toward their profile's total.
- Link chips in the episode drawer open the URL directly instead of the "Most referenced" page.
- The Markdown export doesn't show section labels.
- On SQLite, dates compare as text, so feeds that mix time zones can shift `since`/`until` and first and last seen by a few hours. Day-level filtering is unaffected.
- The ranking query reads the whole links table, which briefly blocks writers on SQLite. Don't call it in a tight loop while the queue is busy.
- The boilerplate thresholds are fixed. They're reported in the response but can't be set through the API.

## Remaining work

### 3b: Podcast Index initial pull (conditional)

If a feed only lists its latest 10–300 episodes, our capture can only count links from those. Podcast Index may still hold descriptions for older episodes. Briefcast already depends on a Podcast Index client and reads `PODCASTINDEX_KEY` / `PODCASTINDEX_SECRET` for search.

1. **Measure first.** Write a read-only script that compares, for each subscribed podcast, how many episodes Podcast Index knows with how many are in our feed and database. If the gap is small, stop here.
2. **If the gap is real:**
   - Add an opt-in Podcast Index backfill per podcast, using the existing credentials. It runs only when triggered, so it makes no hidden network calls.
   - Use Podcast Index descriptions only for episodes missing from our feed.
   - Save their links through the same path as ingest.
3. **Schema (needs approval):**
   - A `source` column (`feed` or `podcastindex`) on `show_note_links`.
   - Episodes known only to Podcast Index stored as reference-only records, without media or downloads.
4. **Ongoing check:** where both sources have an episode, log how many links differ. This is a data-quality signal, not a gate.

**Not yet verified:**
- Whether the client library includes the episodes-by-feed endpoint.
- How far back Podcast Index keeps old episodes.
- Podcast Index's current terms of use.

### 7: Optional extras

- **Follow short links.** Opt-in and off by default, because it makes network calls. Use `service/outbound_url_policy.go` and the host limiter, and resolve each short link once.
- **Merge one person's accounts across platforms**, by hand or with an LLM.
- **Re-extract links when an episode's show notes change.** Today a refresh skips episodes it already has.

## Outside sources considered

- **My Podcast Data:**
  - Free to browse. It has per-show link pages, with counts and the other shows that cite the same link.
  - It has no API or export, and it blocked automated access (403) when checked on 2026-11-05.
  - Its data comes from the same public feeds we read. Don't scrape it.
  - Use it only for the manual spot-check above. Ask through the contact address on its privacy page if an export is ever wanted.
- **Podscan, Listen Notes, Podchaser:** transcript and metadata search with alerts. None was found to aggregate show-note links.
- **Muck Rack:** brand monitoring in podcasts.
- **Podcast Index:** raw episode descriptions, with no link aggregation. It is the source for 3b.

## Rollback

- The new table and columns are additive.
- Turning off the new routes and UI leaves per-episode links working as before.
- The dedupe and orphan migrations deleted rows, so undoing them means restoring a backup taken before the deploy.
