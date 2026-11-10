import { reactive } from "vue";

// The list a reader came from (e.g. the Summaries page), so the episode page can offer
// Previous/Next and the J/K shortcuts through it. It lives for the browser tab only.
const list = reactive({ ids: [] as string[], label: "" });

export function setReadingList(ids: string[], label: string): void {
  list.ids = [...ids];
  list.label = label;
}

export function clearReadingList(): void {
  list.ids = [];
  list.label = "";
}

export interface ReadingPosition {
  previous: string | null;
  next: string | null;
  index: number;
  total: number;
  label: string;
}

export function readingPosition(id: string): ReadingPosition | null {
  const index = list.ids.indexOf(id);
  if (index < 0) {
    return null;
  }
  return {
    previous: index > 0 ? list.ids[index - 1]! : null,
    next: index < list.ids.length - 1 ? list.ids[index + 1]! : null,
    index,
    total: list.ids.length,
    label: list.label,
  };
}
