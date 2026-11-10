import { reactive } from "vue";

// App-wide notifications. Results of user actions go here instead of alerts at the top
// of the page, which shift the layout and are off-screen after scrolling.
// Success and info messages time out; errors stay until dismissed.
export type ToastTone = "success" | "info" | "warning" | "danger";

export interface Toast {
  id: number;
  tone: ToastTone;
  message: string;
}

export const TOAST_TIMEOUT_MS = 6000;
const MAX_TOASTS = 4;

const state = reactive({ toasts: [] as Toast[] });
const timers = new Map<number, ReturnType<typeof setTimeout>>();
let nextId = 1;

export function dismissToast(id: number): void {
  const timer = timers.get(id);
  if (timer !== undefined) {
    clearTimeout(timer);
    timers.delete(id);
  }
  const index = state.toasts.findIndex((toast) => toast.id === id);
  if (index >= 0) {
    state.toasts.splice(index, 1);
  }
}

export function showToast(message: string, tone: ToastTone = "success"): number {
  const text = message.trim();
  if (!text) {
    return 0;
  }
  // The same message twice in a row replaces the first instead of stacking.
  const duplicate = state.toasts.find((toast) => toast.message === text && toast.tone === tone);
  if (duplicate) {
    dismissToast(duplicate.id);
  }
  const id = nextId;
  nextId += 1;
  state.toasts.push({ id, tone, message: text });
  while (state.toasts.length > MAX_TOASTS) {
    dismissToast(state.toasts[0]!.id);
  }
  if (tone !== "danger") {
    timers.set(id, setTimeout(() => dismissToast(id), TOAST_TIMEOUT_MS));
  }
  return id;
}

export const toast = {
  success: (message: string) => showToast(message, "success"),
  info: (message: string) => showToast(message, "info"),
  warning: (message: string) => showToast(message, "warning"),
  error: (message: string) => showToast(message, "danger"),
};

export function useToasts(): { readonly toasts: readonly Toast[] } {
  return state;
}

export function clearToasts(): void {
  for (const toast of [...state.toasts]) {
    dismissToast(toast.id);
  }
}
