// Builds request paths. Every interpolated value is percent-encoded, so an ID can never
// add path segments, a query string or a fragment ("a/delete" -> "a%2Fdelete"), and a
// value of "." or ".." cannot walk up a segment.
function encodeSegment(value: string | number): string {
  const encoded = encodeURIComponent(String(value));
  return encoded === "." || encoded === ".." ? encoded.replace(/\./g, "%2E") : encoded;
}

export function apiPath(strings: TemplateStringsArray, ...values: Array<string | number>): string {
  return strings.reduce((path, part, index) => {
    const value = index < values.length ? encodeSegment(values[index]!) : "";
    return path + part + value;
  }, "");
}

export function podcastImageUrl(podcastId: string): string {
  return apiPath`/podcasts/${podcastId}/image`;
}

export function episodeImageUrl(episodeId: string): string {
  return apiPath`/podcastitems/${episodeId}/image`;
}

export function episodeAudioUrl(episodeId: string): string {
  return apiPath`/podcastitems/${episodeId}/file`;
}

export function episodeMarkdownPackageUrl(episodeId: string, kind: "summary" | "transcript"): string {
  return `${apiPath`/podcastitems/${episodeId}/${kind}`}?format=markdown-package`;
}
