// App (hash-router) paths for records. IDs are encoded so they always stay one segment.
export function podcastPath(podcastId: string): string {
  return `/podcasts/${encodeURIComponent(podcastId)}`;
}

export function episodePath(episodeId: string): string {
  return `/episodes/${encodeURIComponent(episodeId)}`;
}
