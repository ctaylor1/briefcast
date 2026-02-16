import { onUnmounted, watch, type WatchSource } from "vue";

export function useDebouncedWatch<T>(
  source: WatchSource<T> | WatchSource<T>[],
  callback: (value: T, oldValue: T | undefined) => void,
  delayMs = 300,
): void {
  let timer: number | undefined;
  watch(source, (value, oldValue) => {
    if (timer) {
      window.clearTimeout(timer);
    }
    timer = window.setTimeout(() => {
      callback(value as T, oldValue as T | undefined);
    }, delayMs);
  });

  onUnmounted(() => {
    if (timer) {
      window.clearTimeout(timer);
    }
  });
}
