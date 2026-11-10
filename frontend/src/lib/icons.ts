// The app's icon set: 24x24 stroke paths, drawn with currentColor. Add icons here rather
// than inlining SVG in components. `fill: true` icons are solid shapes.
export interface IconDefinition {
  paths: string[];
  fill?: boolean;
}

export const ICONS = {
  // Navigation
  library: { paths: ["M4 6h16", "M4 12h16", "M4 18h10"] },
  episodes: { paths: ["M5 5h14v14H5z", "M10 9l5 3-5 3z"] },
  summaries: { paths: ["M6 3h8l4 4v14H6z", "M14 3v4h4", "M9 13h6", "M9 17h4"] },
  links: {
    paths: [
      "M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1",
      "M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1",
    ],
  },
  activity: { paths: ["M3 12h4l3-8 4 16 3-8h4"] },
  settings: { paths: ["M4 7h16", "M4 17h16", "M9 5v4", "M15 15v4"] },
  info: { paths: ["M12 16v-5", "M12 8h.01", "M12 21a9 9 0 1 1 0-18a9 9 0 0 1 0 18z"] },
  search: { paths: ["M20 20l-4.2-4.2", "M11 18a7 7 0 1 1 0-14a7 7 0 0 1 0 14z"] },
  menu: { paths: ["M4 6h16", "M4 12h10", "M4 18h16"] },
  plus: { paths: ["M12 5v14", "M5 12h14"] },
  "plus-circle": { paths: ["M12 8v8", "M8 12h8", "M12 21a9 9 0 1 1 0-18a9 9 0 0 1 0 18z"] },
  // Playback
  play: { paths: ["M8 5.5v13l11-6.5z"], fill: true },
  pause: { paths: ["M7 5h3.5v14H7z", "M13.5 5H17v14h-3.5z"], fill: true },
  "skip-back": { paths: ["M4 12a8 8 0 1 0 2.3-5.7", "M4 4v5h5"] },
  "skip-forward": { paths: ["M20 12a8 8 0 1 1-2.3-5.7", "M20 4v5h-5"] },
  previous: { paths: ["M6 5v14", "M18 6l-9 6 9 6z"] },
  next: { paths: ["M18 5v14", "M6 6l9 6-9 6z"] },
  queue: { paths: ["M4 6h12", "M4 12h12", "M4 18h8", "M18 14v6", "M15 17h6"] },
  expand: { paths: ["M18 15l-6-6-6 6"] },
  collapse: { paths: ["M6 9l6 6 6-6"] },
  // Actions
  download: { paths: ["M12 4v10", "M8 10l4 4 4-4", "M5 20h14"] },
  "download-resume": { paths: ["M12 3v9", "M8.5 8.5L12 12l3.5-3.5", "M4 15v3a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3", "M10 16.5l2.5 1.5-2.5 1.5z"] },
  "pause-downloads": { paths: ["M9 4v9", "M15 4v9", "M4 15v3a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3"] },
  archive: { paths: ["M4 7h16v4H4z", "M6 11v8h12v-8", "M10 15h4"] },
  "skip-sponsor": { paths: ["M17 8l4-4", "M17 16l4 4", "M3 12h14", "M7 8l-4 4 4 4"] },
  briefpoint: { paths: ["M12 3l8 4.5v9L12 21l-8-4.5v-9z", "M12 12v9", "M12 12L4 7.5", "M12 12l8-4.5"] },
  feed: { paths: ["M5 11a8 8 0 0 1 8 8", "M5 5a14 14 0 0 1 14 14", "M6 19a1 1 0 1 0 0-2 1 1 0 0 0 0 2z"] },
  trash: { paths: ["M3 6h18", "M8 6V4h8v2", "M19 6v14a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V6", "M10 11v6", "M14 11v6"] },
  close: { paths: ["M6 6l12 12", "M18 6L6 18"] },
  check: { paths: ["M5 12.5l4.5 4.5L19 7.5"] },
  "check-circle": { paths: ["M8 12.5l3 3 5-6", "M12 21a9 9 0 1 1 0-18a9 9 0 0 1 0 18z"] },
  circle: { paths: ["M12 21a9 9 0 1 1 0-18a9 9 0 0 1 0 18z"] },
  star: { paths: ["M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3 6.4 20.2l1.1-6.2L3 9.6l6.2-.9z"] },
  "star-filled": { paths: ["M12 3l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3 6.4 20.2l1.1-6.2L3 9.6l6.2-.9z"], fill: true },
  refresh: { paths: ["M20 11a8 8 0 1 0-2.3 5.7", "M20 4v7h-7"] },
  more: { paths: ["M5 12h.01", "M12 12h.01", "M19 12h.01"] },
  external: { paths: ["M14 4h6v6", "M20 4l-9 9", "M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"] },
  "chevron-left": { paths: ["M15 18l-6-6 6-6"] },
  "chevron-right": { paths: ["M9 18l6-6-6-6"] },
  "chevron-down": { paths: ["M6 9l6 6 6-6"] },
  "arrow-left": { paths: ["M19 12H5", "M11 6l-6 6 6 6"] },
  // Theme
  sun: { paths: ["M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10z", "M12 2v2", "M12 20v2", "M4.9 4.9l1.4 1.4", "M17.7 17.7l1.4 1.4", "M2 12h2", "M20 12h2", "M4.9 19.1l1.4-1.4", "M17.7 6.3l1.4-1.4"] },
  moon: { paths: ["M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5z"] },
  monitor: { paths: ["M3 5h18v11H3z", "M8 20h8", "M12 16v4"] },
} satisfies Record<string, IconDefinition>;

export type IconName = keyof typeof ICONS;
