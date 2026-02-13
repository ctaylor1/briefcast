# Briefcast Frontend

Vue 3 + Vite + TypeScript + Tailwind CSS frontend for Briefcast.

## Component system

This app now uses a lightweight design system built from reusable UI primitives in `src/components/ui`:

- `UiButton`
- `UiCard`
- `UiAlert`
- `UiBadge`
- `UiDialog` (Headless UI powered, accessible modal/dialog)

Class composition is centralized via `src/lib/cn.ts` (`clsx` + `tailwind-merge`), so component variants stay consistent.

## Typed API client

Typed endpoint modules are split by domain under `src/lib/api`:

- `http.ts` shared HTTP client and error mapping
- `podcasts.ts` podcast endpoints
- `episodes.ts` episode endpoints
- `discovery.ts` add/search/import endpoints

The route views consume these clients through `src/lib/api.ts`.

## Feature components

Inline route templates were split into feature-level components:

- `src/components/dashboard/*`
- `src/components/episodes/*`
- `src/components/add/*`

## Responsive-first layout

- Mobile-first nav with collapsible menu and overlay in `src/components/AppNav.vue`
- Adaptive views: card layout on small screens, table layout on desktop in:
  - `src/components/dashboard/PodcastCard.vue`
  - `src/components/dashboard/PodcastsTable.vue`
  - `src/components/episodes/EpisodesListItem.vue`
  - `src/components/episodes/EpisodesTable.vue`
- Fluid spacing scale and shell utilities in `src/style.css`
