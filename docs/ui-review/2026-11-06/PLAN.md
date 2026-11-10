# UI review and PR plan

- **Date:** 2026-11-06.
- **Reviewed:** the Vue 3 app in `frontend/`, served at `/app`, on `feat/show-note-link-tracking` @ `69252ad` with uncommitted changes. The legacy `client/` templates are unrouted and were excluded.
- **Method:** source reading and arithmetic. The app was not rendered during the review, so the "Likely" items need a browser check.
- **Line numbers** refer to the reviewed tree. Locate code by symbol when implementing.
- **Status** was last updated on 2026-11-09, for the release that implements the whole plan. Update the Status columns if anything changes.
- This file was recovered from the review session. The findings and plan are as written then. The Status columns and anything marked *(added 2026-11-08)* came later.

## Status at a glance

- **Done:** all 16 PRs, the frontend halves of refactor items P8 and P19, and a component test harness (`@vue/test-utils`, jsdom).
- **Decisions:** all five were answered on 2026-11-09 (see [Decisions](#decisions)).
- **Still needs a person in a browser:**
  - the new accent colours in both themes;
  - real audio playback with media keys and resume;
  - the Settings save bar;
  - summaries rendered from real data.

  Layout was measured in headless Edge against `8939fec`.

## Verdict

The foundation is sound: spacing, radius and type tokens, skeletons, empty states, reduced-motion handling, Headless UI dialogs, labelled inputs, and a good command palette and reader. Three things hold it back:

1. **The player is not a real player.** It has no scrubber or time display, and audio stops when you navigate away.
2. **Dark mode and focus states fail contrast** by a wide margin. *(Fixed by PR 1.)*
3. **The information architecture has drifted.** There is no podcast page, there are two different episode-detail experiences, and pipeline status is scattered across Downloads and Settings.

## Required findings (broken, inaccessible or misleading)

| # | Where | Observed and why it matters | Fix | Confidence | Status |
| --- | --- | --- | --- | --- | --- |
| R1 | `views/PlayerView.vue:458-467`, `:423-456` | `<audio>` has no `controls` and there is no custom scrubber. The user gets Previous, Play, Next and speed only: no seek, elapsed time, skip ±, or volume. `lib/api` has no resume-position call. | Build a transport: scrubber with chapter marks, time, ±15/30 s, speed. | Confirmed | Done |
| R2 | `App.vue:10`, `PlayerView` | The audio element lives inside a routed view, so leaving the Player page stops playback. | Lift audio into the app shell as a persistent player bar. | Confirmed | Done |
| R3 | `views/PlayerView.vue:165-167` | The "Player" nav item opens to a red error ("No items specified for playback") unless a query string is present. | Removed by R2. Until then, show a neutral empty state. | Confirmed | Done |
| R4 | `components/episodes/EpisodeTrackControls.vue:61-73`, `views/EpisodesView.vue:275-283` | "Stop" is enabled only by view-local state that is set after navigating away, so it is disabled on every row. | Remove it; bring it back once R2 provides global playback state. | Confirmed | Done |
| R5 | `components/episodes/EpisodesTable.vue:216`, `views/DownloadsView.vue:660`, `style.css:62` | Both tables had `min-width: 1120px`, but the content column tops out at 1116px. They scrolled sideways at every desktop width (244px at 1200, 164px at 1280, 78px at 1366), cutting off the Actions column. Downloads has no mobile layout. | Drop the min-widths, slim the columns (see D4), and add a card layout for Downloads. | Confirmed by arithmetic | Done (`8cfe2a1` and PR 4) |
| R6 | `style.css:74-85`, `:592-605` | Dark theme never overrode the accent, semantic or badge text colours. Active nav and info badges were 1.89:1, success and warning badges 1.86–1.88:1, the danger badge 2.53:1, and links 3.40:1 (4.5:1 needed). With no `color-scheme`, native selects, date inputs, checkboxes and scrollbars stayed light. | Add a full dark token set and `color-scheme`. | Confirmed (computed) | Done (`209b689`) |
| R7 | `style.css:483-486`, `views/DownloadsView.vue:720-734` | In light mode, the success button (white on `#4dab9a`) was 2.76:1, and the "Paused" and "Download completed" notes about 2.75:1. | Add darker `-strong` variants for text and fills. | Confirmed | Done (`209b689`) |
| R8 | `style.css:136-139` | A global `outline: none` was replaced by a 30%-alpha shadow: 1.45:1 in light, 2.05:1 in dark. It disappears in forced-colors mode and is clipped by `overflow: hidden`. | Use a solid 2px `outline` with `outline-offset`. | Confirmed | Done (`209b689`) |
| R9 | `components/dashboard/PodcastCard.vue:72-180`, `components/dashboard/PodcastsTable.vue:87-196`, `views/SummariesView.vue:814-824` | Eight icon-only buttons per podcast have no accessible name; the tooltip is an unlinked sibling. Toggles lack `aria-pressed`. Summary stars have only `title`. | Create an `IconButton` primitive that requires a label. | Confirmed | Done |
| R10 | `views/SummariesView.vue:298-307` | The reader binds ↑/↓ to previous/next summary, so arrow-key scrolling jumps to another article. | Keep J/K (or ←/→); release ↑/↓. | Confirmed | Done |
| R11 | `components/ui/UiDropdown.vue:25-36`, `components/episodes/EpisodesFilters.vue:98` | Menus built on `<details>` don't close on an outside click, Escape or selection. The top-bar menu stays open after navigating. | Use Headless UI `Menu`/`Popover`, which is already a dependency. | Confirmed | Done |

Paths are relative to `frontend/src/`.

## Recommended findings (design quality)

| # | Area | Observed | Direction |
| --- | --- | --- | --- |
| D1 | Information architecture | **No podcast page:** the cover, title and play button all autoplay the newest episode (`views/DashboardView.vue:60-67`), while the palette sends the same podcast to Episodes. **Unrouted reader:** the summaries reader is local state, so Back leaves the page and there are no deep links. **Two detail views:** the drawer shows the summary as raw `<pre>` (`views/EpisodesView.vue:619-621`); the reader renders Markdown but flattens the transcript. **Scattered pipeline:** status is split across Downloads, Settings › Work Queue and Settings › Logs. | Add a podcast detail route. Build one routed episode detail used everywhere. Create an "Activity" page for the download → transcribe → summarize pipeline and its logs. |
| D2 | Chrome | A top bar `h1` (18px) and non-link breadcrumbs, then a page `h2` (20px) and subtitle on every screen. The hierarchy is inverted, and about 150px goes before any content. Sidebar items carry description lines. "Menu" uses a person icon and duplicates the nav. The 30px page-title token is never used. | One title per page. Drop the breadcrumbs and nav descriptions. Replace Menu with a theme toggle. |
| D3 | Podcast actions | Eight same-weight icons, four of them persistent settings. "Resume downloads" and "Open player" use the same triangle. Delete sits beside the toggles. | One primary action plus an overflow menu; move settings to the podcast page. |
| D4 | Episode rows | Badges for absent state ("Unplayed", "No favorite", "Not queued") repeat what the adjacent toggles show. Each row has five or six 44px buttons plus Details. Mobile cards carry a 170px image each. | Show state only when present. Use a 48px thumbnail, one primary action, and put the rest in a menu. |
| D5 | Feedback | Inline alerts sit at the top of the page, so they shift the layout and are off-screen when you save far down Settings. Success messages on Episodes never clear. There are six native `alert()` calls. | Add a toast region and field-level errors. |
| D6 | Settings | Seven separate Save buttons. Theme autosaves (`composables/useTheme.ts:128-131`) yet also has a Save button. A model chosen in one card must be saved from another (`views/SettingsView.vue:1207`). Raw `<select>` elements bypass `UiSelect`. | Use section navigation and one save model (toggles autosave, text fields save explicitly). Merge the two model cards. |
| D7 | Theme | "Auto" is a clock schedule defaulting to New York time, and `prefers-color-scheme` is ignored. The theme is applied in `onMounted`, so users of the dark theme see a light flash. | Offer System / Light / Dark, with the schedule as an advanced option. Set the theme in an inline script in `index.html`. |
| D8 | System drift | Tailwind and `tailwind-merge` are installed but never imported. Six undefined tokens and `.dialog-panel--wide` are referenced. `.icon-btn`, the status helpers and the podcast filter are each duplicated. About 45 hand-inlined SVGs sit alongside Unicode glyphs (▶ ⏹ ↻). There is no elevation: popovers have only a 1.22:1 border. Control heights are 48/44/40/36/32. | One icon component, elevation tokens and a two-step density scale. Delete the duplicates. |
| D9 | Mobile nav | Seven bottom tabs come to about 48px each at 375px with 11px labels; "Add Podcast" and "Downloads" won't fit. | Cut to four or five destinations (see PR 13). Likely; verify. |
| D10 | Reader toolbar | It is sticky at `top: 0` with z-index 50, over the sticky top bar at z-index 15. Its −16px margins don't match the 24/32px page padding (`views/SummariesView.vue:886-899`). | Offset it by the top-bar height and use the padding token. Likely; verify. |

**Nits:**
- The page `<title>` is "frontend". *(Fixed in PR 3.)*
- `HelloWorld.vue`, `vue.svg` and `vite.svg` are scaffolding leftovers. *(Removed in PR 6.)*
- Pagination says "episodes" on the Summaries page.
- Label casing is mixed ("Download Completed", "Alphabetical Z-A").
- `toLocaleString()` puts seconds in table timestamps.

## PR plan

| # | PR | Findings | Size | Needs | Status |
| --- | --- | --- | --- | --- | --- |
| 1 | Theme tokens: contrast, dark mode, focus ring | R6, R7, R8 | S | — | Done (`209b689`) |
| 2 | Accessible names and pressed states on icon buttons | R9 | S | — | Done |
| 3 | Interaction fixes: reader arrow keys, dead Stop button, Player empty state, pagination label, page title | R3, R4, R10, nits | S | — | Done |
| 4 | Table overflow on desktop; card layout for Downloads on mobile | R5 | S–M | — | Done (`8cfe2a1` and PR 4) |
| 5 | Dismissible menus (Headless UI) and one shared podcast filter | R11 | M | — | Done |
| 6 | Cleanup: scaffolding files, undefined tokens, Tailwind removal | D8 | S | Tailwind decision; changes the lockfile | Done |
| 7 | Primitives: icon, icon button, toast; replace `alert()` and page-top alerts | D5, D8 | M | Icon decision | Done |
| 8 | Theme behaviour: System / Light / Dark, applied before first paint | D7 | S–M | Check the backend accepts a new theme value | Done |
| 9 | Persistent player bar with scrubber, time, skip and speed | R1, R2 | L | Player decision | Done |
| 10 | Now-playing view; resume position | R1 | M–L | New API and schema field: needs approval | Done |
| 11 | Podcast detail page; actions in an overflow menu | D1, D3 | M | — | Done |
| 12 | One routed episode detail replacing the drawer and the reader | D1, D10 | L | — | Done |
| 13 | Activity page; navigation cut to five destinations | D1, D2, D9 | M–L | Navigation decision | Done |
| 14 | Slimmer episode rows | D4 | M | Device decision | Done |
| 15 | Settings restructure and one save model | D6 | M | — | Done |
| 16 | Type scale and brand accent (optional) | — | S | — | Done |

Notes:
- **PR 1:** all text, fill, badge, focus and star pairings pass a numeric contrast check in both themes. A CI test now checks contrast for the PR 16 accent (`#1370ab` light, `#0ea7e0` dark).
- **PR 4:** Downloads shows cards below 1200px. Measured sideways scroll went from 674px at 375 and 76px at 1280 to 0 everywhere.
- **PR 8:** the stored theme value `auto`, which is the database default, now means "follow the system". The clock schedule is an explicit "Schedule" mode.
- **PR 9–10:** playback survives navigation. The resume position adds a nullable `playback_position_seconds` column and `PATCH /podcastitems/:id/position`.
- **PR 12:** one routed episode page replaces the drawer and the Summaries reader. J/K still works from Summaries.
- **PR 13:**
  - Navigation is Library · Episodes · Summaries · Activity · Settings. "Most referenced" sits under Library.
  - `/downloads` and `/about` redirect to Activity and Settings, and unknown routes go to Library.
  - On phones, the bottom tabs went from 7 at 48px to 5 at 69px.
- **PR 14:** median row height went from 549px to 102px on a phone, and from 169px to 65px at 1280.
- **PR 15:** this also fixed a bug that predated the plan: every field shared one id, so labels pointed at the wrong fields.
- **Removed views:** the Library table view and the in-page Summaries reader.

### Phases

The PRs follow these phases. The order matters: build the primitives before redesigning screens.

| Phase | Deliverable | PRs |
| --- | --- | --- |
| 0. Baseline | Get the app running; screenshot each view at 375, 768 and 1280px in both themes; verify the "Likely" items; decide on Tailwind; delete the scaffolding. | 6 |
| 1. Repair | Tokens, focus ring, accessible names, table overflow, arrow keys, dismissible menus, dead Stop, neutral Player empty state. | 1–5 |
| 2. Primitives | `UiIcon`, `IconButton`, `Menu`, `Tabs`, `Toast`, a shared `PodcastFilter` and status helpers, elevation and density tokens, theme bootstrap. | 7, 8 |
| 3. Player | Persistent player bar with scrubber, chapter marks, skip, time and Media Session; an expanded "Now playing" view with chapters and transcript. | 9, 10 |
| 4. Structure | New navigation, podcast page, routed episode detail, Activity page. Routes change, so old hash URLs need redirects. | 11–13 |
| 5. Screens | Library as a cover grid, slimmer episode rows, sectioned Settings, Downloads cards on mobile. | 4, 14, 15 |
| 6. Identity (optional) | A real type scale, and an accent aligned to the logo gradient (`#0ea7e0` → `#1a2d6b`). | 16 |

## Decisions

All five were answered on 2026-11-09:

1. **Player scope:** build the persistent player and resume position, including the schema change.
2. **Navigation:** five destinations. Work Queue and Logs move to Activity; Add, Player and About leave the nav.
3. **Tailwind:** removed.
4. **Icons:** a local `UiIcon` with a curated path set.
5. **Primary device:** desktop first; phone layouts still work.

## Verification

- Run the frontend commands in `AGENTS.md` (`test:unit`, then `build`) for every PR. PRs 2–5 change templates and TypeScript.
- Component tests run with `@vue/test-utils` and jsdom under the same `test:unit` command.
- Check each visual PR in a browser in both themes at 375, 768 and 1280px before merging.

## Do not do

- Rewrite into Tailwind or a component library mid-stream.
- Redesign screens before the Phase 2 primitives exist.
- Change backend routes as part of UI work.
