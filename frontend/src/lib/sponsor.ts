import type { Chapter } from "../types/api";

const sponsorMatchers: RegExp[] = [
  /\b(ad|ads|advert|advertisement)\b/i,
  /\bsponsor(ed|ship)?\b/i,
  /\bpromo\b/i,
  /\bpromotion(al)?\b/i,
  /\bbreak\b/i,
  /\bsupported by\b/i,
  /\bbrought to you by\b/i,
  /\bpartner\b/i,
];

export type SponsorSegment = {
  start: number;
  end: number;
  title: string;
};

export function isSponsorChapter(title: string): boolean {
  const trimmed = title.trim();
  if (!trimmed) {
    return false;
  }
  return sponsorMatchers.some((matcher) => matcher.test(trimmed));
}

export function buildSponsorSegments(chapters: Chapter[], duration?: number): SponsorSegment[] {
  if (!chapters || chapters.length === 0) {
    return [];
  }
  const sorted = [...chapters].sort((a, b) => a.startSeconds - b.startSeconds);
  const segments: SponsorSegment[] = [];
  for (let i = 0; i < sorted.length; i += 1) {
    const chapter = sorted[i];
    if (!chapter) {
      continue;
    }
    if (!isSponsorChapter(chapter.title)) {
      continue;
    }
    const start = Math.max(0, Number(chapter.startSeconds) || 0);
    let end = chapter.endSeconds ?? 0;
    if (!end || end <= start) {
      const next = sorted[i + 1];
      if (next && Number.isFinite(next.startSeconds)) {
        end = next.startSeconds;
      } else if (duration && duration > start) {
        end = duration;
      } else {
        end = start;
      }
    }
    if (end > start) {
      segments.push({ start, end, title: chapter.title });
    }
  }
  return segments;
}
