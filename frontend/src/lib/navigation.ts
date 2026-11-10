import type { IconName } from "./icons";

export interface NavItem {
  to: string;
  label: string;
  icon: IconName;
  // Shown in the command palette, not in the sidebar.
  meta: string;
  keywords: string;
}

// The five destinations: sidebar, mobile bottom tabs and command palette.
export const primaryNavItems: NavItem[] = [
  { to: "/", label: "Library", icon: "library", meta: "Your podcasts", keywords: "podcasts dashboard home feeds subscriptions library" },
  { to: "/episodes", label: "Episodes", icon: "episodes", meta: "Every episode, searchable", keywords: "episodes search chapters transcript filters" },
  { to: "/summaries", label: "Summaries", icon: "summaries", meta: "AI summaries to read", keywords: "summaries ai llm read browse" },
  { to: "/activity", label: "Activity", icon: "activity", meta: "Downloads, transcription, logs", keywords: "activity downloads queue transcription work logs monitoring pipeline" },
  { to: "/settings", label: "Settings", icon: "settings", meta: "Retention, summaries, appearance", keywords: "settings preferences retention defaults theme about" },
];

// Shown under Library in the sidebar; reachable from the Library page on phones.
export const librarySubItems: NavItem[] = [
  { to: "/links", label: "Most referenced", icon: "links", meta: "Links your podcasts cite most", keywords: "links references show notes mentions people articles posts ranking top" },
];

// Command palette only: places that are actions or live elsewhere in the layout now.
export const paletteOnlyItems: NavItem[] = [
  { to: "/add", label: "Add podcast", icon: "plus-circle", meta: "By URL, OPML or search", keywords: "add import discover rss feed url opml subscribe" },
  { to: "/player", label: "Now playing", icon: "play", meta: "The player and its queue", keywords: "player playback listen now playing queue" },
  { to: "/activity", label: "Downloads", icon: "download", meta: "Activity › Downloads", keywords: "downloads queue progress pause cancel" },
  { to: "/settings?section=about", label: "About", icon: "info", meta: "Version and source code", keywords: "about version github source repository release" },
];

export const commandNavItems: NavItem[] = [...primaryNavItems, ...librarySubItems, ...paletteOnlyItems];

// The nav item a path belongs to (for aria-current and the page title).
export function activeNavItem(path: string): NavItem | undefined {
  const all = [...librarySubItems, ...primaryNavItems];
  return all.find((item) => (item.to === "/" ? path === "/" || path.startsWith("/podcasts") : path === item.to || path.startsWith(`${item.to}/`)))
    ?? (path.startsWith("/links") ? librarySubItems[0] : undefined);
}
