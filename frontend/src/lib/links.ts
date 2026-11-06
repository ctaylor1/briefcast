import type { TopLinksQuery } from "./api/links";
import type { LinkTargetSummary, ShowNoteLink } from "../types/api";

export interface LinkFilters {
  kind: string;
  platform: string;
  podcastId: string;
  since: string;
  until: string;
  includeBoilerplate: boolean;
}

export const DEFAULT_LINK_FILTERS: Readonly<LinkFilters> = {
  kind: "",
  platform: "",
  podcastId: "",
  since: "",
  until: "",
  includeBoilerplate: false,
};

const KIND_LABELS: Record<string, string> = {
  profile: "Profile",
  post: "Post",
  shortener: "Short link",
  web: "Web page",
};

const PLATFORM_LABELS: Record<string, string> = {
  x: "X",
  instagram: "Instagram",
  tiktok: "TikTok",
  linkedin: "LinkedIn",
  youtube: "YouTube",
  threads: "Threads",
  bluesky: "Bluesky",
  web: "Web",
};

export const LINK_KIND_OPTIONS = Object.entries(KIND_LABELS).map(([value, label]) => ({ value, label }));
export const LINK_PLATFORM_OPTIONS = Object.entries(PLATFORM_LABELS).map(([value, label]) => ({ value, label }));

export function linkKindLabel(kind: string): string {
  return KIND_LABELS[kind] ?? kind;
}

export function linkPlatformLabel(platform: string): string {
  return PLATFORM_LABELS[platform] ?? platform;
}

export function pluralize(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

/**
 * Returns the URL only if it is safe to use as an href. Show note URLs come
 * from third-party feeds, so anything but http(s), such as javascript: or
 * data:, must never reach an anchor.
 */
export function safeExternalUrl(raw: string | null | undefined): string | null {
  if (!raw) {
    return null;
  }
  try {
    const parsed = new URL(raw);
    return parsed.protocol === "http:" || parsed.protocol === "https:" ? parsed.href : null;
  } catch {
    return null;
  }
}

function displayUrl(url: string): string {
  return url
    .replace(/^https?:\/\//i, "")
    .replace(/^www\./i, "")
    .replace(/\/$/, "");
}

function formatHandle(platform: string, handle: string | undefined): string {
  if (!handle) {
    return "";
  }
  // LinkedIn profiles are URL slugs, not @handles.
  return platform === "linkedin" ? handle : `@${handle.replace(/^@/, "")}`;
}

export function linkTargetLabel(
  target: Pick<LinkTargetSummary, "kind" | "platform" | "handle" | "canonicalUrl">,
): string {
  const handle = formatHandle(target.platform, target.handle);
  if (handle && target.kind === "profile") {
    return handle;
  }
  if (handle && target.kind === "post") {
    return `Post by ${handle}`;
  }
  return displayUrl(target.canonicalUrl);
}

export function showNoteLinkLabel(link: Pick<ShowNoteLink, "Title" | "URL">): string {
  return link.Title.trim() || displayUrl(link.URL);
}

export interface LinkSection {
  key: string;
  heading: string;
  links: ShowNoteLink[];
}

/**
 * Groups links under their show note section, keeping the order in which
 * sections and links first appear.
 */
export function groupLinksBySection(links: ShowNoteLink[]): LinkSection[] {
  const grouped = new Map<string, ShowNoteLink[]>();
  for (const link of links) {
    const label = link.ContextLabel.trim();
    const section = grouped.get(label);
    if (section) {
      section.push(link);
    } else {
      grouped.set(label, [link]);
    }
  }
  const unlabeledHeading = grouped.size > 1 ? "Other links" : "Links";
  return Array.from(grouped, ([label, sectionLinks]) => ({
    key: label,
    heading: label || unlabeledHeading,
    links: sectionLinks,
  }));
}

export function buildTopLinksQuery(filters: LinkFilters): TopLinksQuery {
  const query: TopLinksQuery = {};
  if (filters.kind) {
    query.kind = filters.kind;
  }
  if (filters.platform) {
    query.platform = filters.platform;
  }
  if (filters.podcastId) {
    query.podcastId = filters.podcastId;
  }
  if (filters.since) {
    query.since = filters.since;
  }
  if (filters.until) {
    query.until = filters.until;
  }
  if (filters.includeBoilerplate) {
    query.includeBoilerplate = true;
  }
  return query;
}

export function hasActiveLinkFilters(filters: LinkFilters): boolean {
  return (Object.keys(DEFAULT_LINK_FILTERS) as Array<keyof LinkFilters>).some(
    (key) => filters[key] !== DEFAULT_LINK_FILTERS[key],
  );
}
