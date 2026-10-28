# Podcast Markdown exports

Briefcast implements the supplied **podcast-markdown 1.2** contract for its existing
transcript and summary outputs. Compiled `source_note` documents are not generated.
This is a project schema, not an industry standard. Files use UTF-8, quoted JSON
values in YAML frontmatter, and CommonMark-compatible bodies. No vault or provider
is required to interpret the files. The frontmatter convention and body syntax are
documented by [Jekyll](https://jekyllrb.com/docs/front-matter/) and
[CommonMark](https://spec.commonmark.org/).

## Output and identity

- Original directory layout is retained: `DATA/markdown/transcripts/<podcast>/<year>/<year-month>/`
  and `DATA/markdown/summaries/<podcast>/<year>/<year-month>/`.
- The first available `<episode>.md` is written once. If it already exists with
  different bytes, a new `<episode>.r<revision>.<content-digest>.<collision-number>.md`
  is created. Existing legacy files and manual edits are never overwritten.
- Long filenames reserve room for revision suffixes within the filesystem's
  255-byte component limit. A digest distinguishes truncated names; full titles
  and stable identities remain in frontmatter.
- Stable UUID URNs derive from the internal episode UUID and document type, not
  filenames, titles, library ordering, RSS GUIDs or filesystem paths. All files for
  an episode share `episode_id`; transcript and summary IDs differ.
- Every changed source/body/retained metadata snapshot increments the document
  revision. Identical repeated exports return the same bytes and revision.
- Immutable document revisions are retained in the `markdown_revisions` database
  table. `summary_provenance_json` is an additive column on `podcast_items`.
  The normal startup AutoMigrate creates this storage; no bulk conversion is run
  on startup. Back up the database before deploying as for other schema additions.
- `created` and `updated` describe the newly authored portable Markdown documents.
  They are never copied from DB import time, filesystem times or publication time.
  Source production dates are recorded only when known in their dedicated fields.
- New revisions preserve unfamiliar frontmatter from the preceding stored document
  and its tracked exported file. Edited files remain untouched. Invalid edited YAML
  stops that export with an error rather than discarding metadata. Manually moved
  files remain self-identifying; the app cannot automatically discover external edits
  after a file has been moved outside its tracked export path.
- Initial conversion also preserves unfamiliar frontmatter in an existing legacy
  file at the original export path. Episode deletion removes its private database
  revision history transactionally; database migration copies revision history when
  the source database has it, while supporting older source databases without it.

Transcript `.md` bodies use the same plain text as `.txt` exports: existing
paragraphs and speaker labels are preserved, without added segment sections,
timestamps, bold markers, backslash escaping, or code fences. Only the frontmatter
is YAML. Original ASR timestamps remain in the hashed source companions; the plain
body records `timestamp_granularity: "none"`.

Plain `.txt` exports retain their previous content and paths. Summary Markdown
contains the actual summary body, rather than appending show notes that were not
read by the summarizer. Plain summary exports continue to include show notes/links.

## Inputs and provenance

New summaries retain the requested model and the provider/runtime, the actual
returned model only when exposed, the successful completion time, relevant request
settings, and **both** exact system and user-template strings. `prompt_sha256` is
the SHA-256 of the UTF-8 compact JSON representation of the `prompt` mapping
(`system` and `user_template`, lexically sorted keys, using Go `encoding/json`).
`prompt_id` is content-addressed; `prompt_version` is its digest. No credentials,
private runtime endpoint or current environment configuration are exported.

The summarizer reads the plain transcript body, without added segment or paragraph
labels. Summary bodies retain their Markdown headings, lists, emphasis, and links. The exact text
actually sent to the model is retained in a companion `inputs/<sha256>.txt` file,
referenced in its `inputs` record with a content-addressed snapshot ID/revision. This snapshot is distinct from the source
Markdown document: its hash describes the actual model input, not an unread `.md`
file. `source_document` identifies the exact source Markdown revision/hash;
`source_provenance` retains its known ancestry. The transcript frontmatter itself is
not sent to the LLM. Requests truncated at the existing size limit record partial
coverage and byte locators; slicing preserves UTF-8. Full input coverage describes
the supplied transcript only. Episode coverage remains unknown unless established.
Inherited source gaps remain visible in the summary's own `provenance_missing`.
The preparation event also retains the exact source Markdown snapshot and describes
body extraction and truncation, separately from the model's summarization event.

Detailed provenance is stored separately from Markdown, under
`DATA/provenance/podcast-markdown/` (normally
`E:\Podcasts\provenance\podcast-markdown\` on this installation):

```text
markdown/transcripts/<podcast>/<year>/<month>/<episode>.md
markdown/summaries/<podcast>/<year>/<month>/<episode>.md
provenance/podcast-markdown/<document-uuid>.r<revision>.<digest>.json
provenance/podcast-markdown/inputs/<sha256>.txt
```

Generator 1.2.3 emits concise frontmatter with a `provenance` mapping containing
one portable relative `uri` and the SHA-256 of that exact JSON file. Detailed
`processing_events`, the snapshot index, full prompts/settings, and extended input
ancestry live in the JSON companion. Essential episode metadata, identities,
production times/models, prompt identifiers/hashes, input IDs/revisions/hashes and
coverage, and declared historical gaps remain in the note. The JSON has its own
`provenance_schema: "briefcast-podcast-provenance"`, `schema_version: "1"`, and a
complete `metadata` record. Snapshot payloads are separate content-addressed files.
The JSON never includes its own final-byte hash or raw snapshot text.

Both artifacts and exact input companions are checked and published before the
new Markdown revision is committed. Missing or altered provenance stops re-export,
including unchanged/cached requests. Prior inline revisions are supported; their
history and exact input bytes are retained when making a new version. Older files
and their original companion directories are preserved rather than deleted.

Use **Download Markdown package** in the transcript/summary UI to export a ZIP
containing the note, its JSON provenance, and required input companions in the
same separate directory layout. Extract and move those directories together to
preserve relative references. The existing single-note Obsidian action copies the
readable note only; use the package when full provenance must accompany it. A
single note moved to another directory needs its relative references updated or
the original directory layout preserved. No machine-specific absolute paths are
embedded in the files.

Versions 1.2.1 and 1.2.2 introduced external input payloads and plain transcript
bodies. Version 1.2.3 moves all new provenance and input artifacts out of the
Markdown folders. Re-export creates a new document revision while preserving
source text, original production times, IDs, unfamiliar metadata and event history.
It does not rerun transcription or summarization.

New WhisperX runs capture the installed WhisperX and faster-whisper versions,
ASR model identifier, successful ASR completion time, and original media hash.
The audio is hashed before and after processing; a changed source fails the run.
Preprocessing, ASR, alignment and diarization are separate events with completion
times, input/output snapshots, tools, versions and relevant settings. The original
ASR event is retained after alignment/diarization. Media URIs refer to the original
audio URL, or an assets-relative path when available, never a machine's absolute
download path. PCM digests identify the actual concatenated decoded chunks; the
preprocessing record describes how they were produced. ASR model weight revisions
and transient in-memory PCM locations are not fabricated; unavailable locations
are declared as provenance gaps. Model weight revisions
are omitted when the runtime does not expose them. Resume checkpoints from earlier
runs have unknown historical production details, explicitly listed as a provenance
gap; today's runtime is not attributed to those earlier segments.
Finishing an already complete checkpoint performs no new ASR: it records a
checkpoint import and any actual postprocessing, and leaves original ASR time,
model and installed-version fields unknown.

Known episode webpage URLs are separate from media/feed URLs. Publisher episode
numbers come from retained feed metadata, not Briefcast's library sequence.
Explicit people roles are used where available; a feed author is not assumed to
be a host. Missing people/language facts are omitted, not fabricated. Summary
language is omitted because the chat API does not reliably report output language.

Legacy exports preserve known completion times, model identifiers, system prompts,
WhisperX versions and episode facts. Missing requested/resolved-model distinctions,
user templates, hashes, input revisions or ASR completion times remain unknown.
`provenance_status` and `provenance_missing` expose these gaps. An imported
transcript is `transcription_method: "unknown"`, not presumed human-produced.
Re-exporting does not retranscribe, resummarize, or change historical production
times. `CanonicalUpdatedAt` and old pipeline `generated_at` are not substituted for
the unavailable original ASR completion time.

## UI exports

Transcript and summary API responses include additive `markdown`,
`markdownFileName`, and (on export failure) `markdownError` fields. Existing content
fields are unchanged. Obsidian actions pass these same server-authored documents
through `obsidian://new`, with a document-ID/revision filename and no overwrite
flag. Long notes download as `.md` files instead of exceeding the URI size limit.

## Validation, examples, and templates

The encoder parses every emitted YAML header and validates it before publishing.
It checks required fields/types, dates, UUID URNs, production-method requirements,
declared legacy gaps, exact embedded-input/prompt hashes, processing events and
type-specific review status. YAML duplicate keys, executable/custom tags, anchors
and aliases are rejected. No final-byte hash is stored inside its own file; prior
revision and input hashes are separate records. Newly authored revisions are
`unreviewed`; previous review claims are never copied onto changed content.

Supported templates are [transcript.md.tmpl](transcript.md.tmpl) and
[summary.md.tmpl](summary.md.tmpl). Their placeholders are authoring instructions,
not source facts. Only include optional fields when known. Representative examples
in [examples](examples/) use **fictional test data**, not production claims, and are
validated by the Go test suite. They include a new ASR transcript, a new summary,
and a legacy transcript with explicit gaps.

```powershell
go test ./internal/podcastmarkdown ./service ./controllers ./db
npm --prefix frontend run test:unit
npm --prefix frontend run build
uv run --no-sync --offline pytest
uv run --no-sync --offline ruff check scripts/podcast_provenance.py scripts/whisperx_transcribe.py tests/test_podcast_provenance.py tests/test_whisperx_provenance.py
```

## Repairing existing exports: choices, not automatic migration

1. **Re-export from the database (recommended for unedited files).** After deploying
   and backing up the database/assets, use the existing `POST /settings/export-all`
   API and monitor `GET /settings/export-all`. This now creates conforming Markdown
   alongside originals when needed. It also refreshes the existing plain `.txt`
   exports and may backfill legacy canonical transcript fields, as before. Known
   database metadata is included; unavailable history is listed as partial. This
   makes no LLM requests and runs no ASR. Do not use this as an in-place editor for
   manually authored notes. Failures are logged with episode IDs and returned by
   the service function. The HTTP status endpoint reports whether the job is
   running; inspect server logs for errors and successful export counts.
2. **Repair selected copies while retaining edited bodies.** A separate, reviewed
   repair pass can join existing files to a database snapshot using stable IDs or
   verified GUID/episode matches, preserve unfamiliar YAML and the existing body,
   and write new revisions into a review directory. Produce a match/gap report
   first; do not match by title alone. This targeted repair tool is not part of
   this change and has not been run.
3. **Regenerate content.** Retranscribe or resummarize selected episodes using the
   new pipeline. These are new production events with newly captured provenance,
   not repairs to unverifiable old history. Expect ASR compute and/or LLM API costs.

Bulk conversion is explicit rather than automatic. Tests use isolated temporary
databases and directories; operational re-exports must preserve existing files.

## Operational limits and rollback

Monitor structured `failed to export ... Markdown` and UI export errors, plus
`export_all` failure counts/status. Revision history and input companions increase
storage use. Completed temporary files are published using an atomic
non-replacing hard link; the assets filesystem must support hard links. Unsupported
filesystems fail visibly rather than risk overwriting a note. Copying an export for full input reconstruction must preserve its referenced
`provenance/podcast-markdown` files and input directory, or use the ZIP package. Companion hashes are checked before publishing a new revision;
missing or altered historical companions stop the export rather than losing evidence.

Rollback application code without dropping the additive column/table. Older code
ignores them. Preserve/export the new `.md` revisions first: the old exporter can
overwrite its original `<episode>.md` path, so do not run old exports against the
same assets without a backup. Return to this branch for corrections before merging
the changes into `main` and then into the WhisperX separation branch.
