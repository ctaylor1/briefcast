export type NavIcon = "podcasts" | "episodes" | "summaries" | "downloads" | "add" | "player" | "settings" | "about";

export interface NavItem {
  to: string;
  label: string;
  icon: NavIcon;
  section: string;
  meta: string;
  keywords: string;
}

export interface NavSection {
  title: string;
  items: NavItem[];
}

// Items shown in both the sidebar and the mobile bottom tabs.
export const primaryNavItems: NavItem[] = [
  {
    to: "/",
    label: "Podcasts",
    icon: "podcasts",
    section: "Library",
    meta: "Subscribed feeds",
    keywords: "dashboard home feeds subscriptions library",
  },
  {
    to: "/episodes",
    label: "Episodes",
    icon: "episodes",
    section: "Library",
    meta: "Chapters, transcripts, filters",
    keywords: "episodes search chapters transcript filters",
  },
  {
    to: "/summaries",
    label: "Summaries",
    icon: "summaries",
    section: "Library",
    meta: "Browse AI summaries",
    keywords: "summaries ai llm read browse library",
  },
  {
    to: "/downloads",
    label: "Downloads",
    icon: "downloads",
    section: "Library",
    meta: "Queue status and progress",
    keywords: "downloads queue status progress pause cancel",
  },
  {
    to: "/add",
    label: "Add Podcast",
    icon: "add",
    section: "Library",
    meta: "Add by URL, OPML, discovery",
    keywords: "add import discover rss feed url opml",
  },
  {
    to: "/player",
    label: "Player",
    icon: "player",
    section: "Playback",
    meta: "Now playing queue",
    keywords: "audio player playback listen now playing",
  },
  {
    to: "/settings",
    label: "Settings",
    icon: "settings",
    section: "System",
    meta: "Retention and defaults",
    keywords: "settings preferences retention defaults",
  },
];

// Sidebar and command palette only; kept out of the mobile bottom tabs.
export const aboutNavItem: NavItem = {
  to: "/about",
  label: "About",
  icon: "about",
  section: "System",
  meta: "Repo and running version",
  keywords: "about version github source repository release",
};

// All routable items, in sidebar order: About follows Settings.
export const routeNavItems: NavItem[] = [...primaryNavItems, aboutNavItem];

export function groupNavSections(items: NavItem[]): NavSection[] {
  const grouped = new Map<string, NavItem[]>();
  for (const item of items) {
    if (!grouped.has(item.section)) {
      grouped.set(item.section, []);
    }
    grouped.get(item.section)?.push(item);
  }
  return Array.from(grouped.entries()).map(([title, sectionItems]) => ({ title, items: sectionItems }));
}
