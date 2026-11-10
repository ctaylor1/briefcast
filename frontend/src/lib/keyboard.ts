export type ReaderKeyAction = "next" | "previous" | "close" | null;

export interface KeyLike {
  key: string;
  altKey?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
  defaultPrevented?: boolean;
  target?: EventTarget | null;
}

// True for text fields and other places where typing must not trigger shortcuts.
export function isEditableTarget(target: EventTarget | null | undefined): boolean {
  if (typeof HTMLElement === "undefined" || !(target instanceof HTMLElement)) {
    return false;
  }
  if (target.isContentEditable) {
    return true;
  }
  const tagName = target.tagName;
  return tagName === "INPUT" || tagName === "TEXTAREA" || tagName === "SELECT";
}

// Shortcuts for reading one item after another: J next, K previous, Escape close.
// The arrow keys are deliberately not shortcuts, so they keep scrolling the page.
export function readerKeyAction(event: KeyLike): ReaderKeyAction {
  if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) {
    return null;
  }
  if (isEditableTarget(event.target)) {
    return null;
  }
  switch (event.key) {
    case "j":
    case "J":
      return "next";
    case "k":
    case "K":
      return "previous";
    case "Escape":
      return "close";
    default:
      return null;
  }
}
