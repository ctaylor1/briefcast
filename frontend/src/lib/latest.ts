// Latest-wins guard for fetches that can overlap (typing in a search box, paging quickly,
// switching items). Only the most recent call may apply its result.
//
//   const latest = createLatest();
//   const result = await latest.run(() => api.list(query));
//   if (result.stale) return;      // a newer call started meanwhile
//   items.value = result.value;
export interface LatestResult<T> {
  stale: boolean;
  value: T;
}

export interface Latest {
  run<T>(task: () => Promise<T>): Promise<LatestResult<T>>;
  // Marks any call in flight as stale, e.g. when the view is reset.
  invalidate(): void;
}

export function createLatest(): Latest {
  let generation = 0;
  return {
    async run<T>(task: () => Promise<T>): Promise<LatestResult<T>> {
      generation += 1;
      const mine = generation;
      try {
        const value = await task();
        return { stale: mine !== generation, value };
      } catch (error) {
        if (mine !== generation) {
          // A newer call owns the outcome; this failure no longer matters.
          return { stale: true, value: undefined as T };
        }
        throw error;
      }
    },
    invalidate(): void {
      generation += 1;
    },
  };
}
