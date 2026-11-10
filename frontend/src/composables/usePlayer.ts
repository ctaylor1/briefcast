import { computed, reactive, watch } from "vue";
import { episodesApi, getErrorMessage, podcastsApi } from "../lib/api";
import { episodeAudioUrl, episodeImageUrl } from "../lib/api/paths";
import { buildSponsorSegments } from "../lib/sponsor";
import type { Chapter, PodcastItem } from "../types/api";

// One player for the whole app. The <audio> element lives in the persistent player bar
// (attach), so playback continues while the user moves between pages.

export const SPEED_OPTIONS = [0.75, 0.9, 1, 1.1, 1.2, 1.3, 1.4, 1.5, 1.7, 2, 2.2, 2.5, 2.7] as const;
export const SKIP_BACK_SECONDS = 15;
export const SKIP_FORWARD_SECONDS = 30;
// "Previous" restarts the episode when more than this much has played.
const RESTART_THRESHOLD_SECONDS = 3;
const RATE_KEY = "briefcast-playback-rate";

function readRate(): number {
  try {
    const value = Number(window.localStorage.getItem(RATE_KEY));
    return (SPEED_OPTIONS as readonly number[]).includes(value) ? value : 1;
  } catch {
    return 1;
  }
}

function sortNewestFirst(items: PodcastItem[]): PodcastItem[] {
  return [...items].sort((left, right) => {
    const a = Date.parse(left.PubDate);
    const b = Date.parse(right.PubDate);
    return Number.isNaN(a) || Number.isNaN(b) ? 0 : b - a;
  });
}

const state = reactive({
  queue: [] as PodcastItem[],
  index: -1,
  playing: false,
  buffering: false,
  currentTime: 0,
  duration: 0,
  rate: readRate(),
  chapters: [] as Chapter[],
  error: "",
  autoplayBlocked: false,
  loadingQueue: false,
});

let audio: HTMLAudioElement | null = null;
let detachListeners: (() => void) | null = null;
let lastAutoSkipStart: number | null = null;
// Latest wins: a track load that has been superseded stops before it touches the element.
let loadGeneration = 0;

const current = computed<PodcastItem | null>(() => state.queue[state.index] ?? null);
const hasNext = computed(() => state.index >= 0 && state.index < state.queue.length - 1);
const hasPrevious = computed(() => state.index > 0);
const sponsorSegments = computed(() =>
  buildSponsorSegments(state.chapters, state.duration || current.value?.Duration),
);
const currentSponsorSegment = computed(
  () => sponsorSegments.value.find((s) => state.currentTime >= s.start && state.currentTime < s.end) ?? null,
);
const autoSkipEnabled = computed(() => current.value?.Podcast?.AutoSkipSponsorChapters ?? false);

function finite(value: number): number {
  return Number.isFinite(value) && value > 0 ? value : 0;
}

function clampTime(seconds: number): number {
  const max = finite(state.duration);
  const value = Math.max(0, seconds);
  return max > 0 ? Math.min(value, max) : value;
}

// --- Media Session (lock screen, headset and keyboard media keys) -----------------------

function mediaSession(): MediaSession | null {
  return typeof navigator !== "undefined" && "mediaSession" in navigator ? navigator.mediaSession : null;
}

function updateMediaMetadata(): void {
  const session = mediaSession();
  const item = current.value;
  if (!session || typeof MediaMetadata === "undefined") {
    return;
  }
  session.metadata = item
    ? new MediaMetadata({
        title: item.Title,
        artist: item.Podcast?.Title ?? "",
        album: "Briefcast",
        artwork: [{ src: episodeImageUrl(item.ID) }],
      })
    : null;
}

function updatePositionState(): void {
  const session = mediaSession();
  const duration = finite(state.duration);
  if (!session || typeof session.setPositionState !== "function" || duration <= 0) {
    return;
  }
  try {
    session.setPositionState({
      duration,
      playbackRate: state.rate,
      position: Math.min(Math.max(0, state.currentTime), duration),
    });
  } catch {
    // Some browsers reject transient states; the next update corrects it.
  }
}

function registerMediaSessionHandlers(): void {
  const session = mediaSession();
  if (!session) {
    return;
  }
  const handlers: Array<[MediaSessionAction, MediaSessionActionHandler]> = [
    ["play", () => void play()],
    ["pause", () => pause()],
    ["seekbackward", (details) => skip(-(details.seekOffset ?? SKIP_BACK_SECONDS))],
    ["seekforward", (details) => skip(details.seekOffset ?? SKIP_FORWARD_SECONDS)],
    ["previoustrack", () => void previous()],
    ["nexttrack", () => void next()],
    [
      "seekto",
      (details) => {
        if (typeof details.seekTime === "number") {
          seek(details.seekTime);
        }
      },
    ],
  ];
  for (const [action, handler] of handlers) {
    try {
      session.setActionHandler(action, handler);
    } catch {
      // Not every browser supports every action.
    }
  }
}

// --- Element events ---------------------------------------------------------------------

function onTimeUpdate(): void {
  if (!audio) {
    return;
  }
  state.currentTime = audio.currentTime;
  updatePositionState();
  if (!autoSkipEnabled.value) {
    return;
  }
  const segment = currentSponsorSegment.value;
  if (!segment) {
    lastAutoSkipStart = null;
    return;
  }
  if (lastAutoSkipStart === segment.start) {
    return;
  }
  lastAutoSkipStart = segment.start;
  seek(segment.end);
}

function onDuration(): void {
  if (audio) {
    state.duration = finite(audio.duration) || current.value?.Duration || 0;
  }
}

function onEnded(): void {
  const item = current.value;
  if (item && !item.IsPlayed) {
    item.IsPlayed = true;
    episodesApi.setPlayed(item.ID, true).catch(() => {
      // Marking played is best effort; playback carries on.
    });
  }
  if (hasNext.value) {
    void loadTrack(state.index + 1);
  } else {
    state.playing = false;
  }
}

function onError(): void {
  if (!audio || !audio.getAttribute("src")) {
    return;
  }
  state.playing = false;
  state.buffering = false;
  state.error = "This episode could not be played. The file may be missing or still downloading.";
}

export function attach(element: HTMLAudioElement): () => void {
  detachListeners?.();
  audio = element;
  audio.playbackRate = state.rate;
  const listeners: Array<[string, () => void]> = [
    ["timeupdate", onTimeUpdate],
    ["loadedmetadata", onDuration],
    ["durationchange", onDuration],
    ["play", () => (state.playing = true)],
    ["pause", () => (state.playing = false)],
    ["waiting", () => (state.buffering = true)],
    ["playing", () => (state.buffering = false)],
    ["canplay", () => (state.buffering = false)],
    ["ended", onEnded],
    ["error", onError],
  ];
  for (const [type, listener] of listeners) {
    element.addEventListener(type, listener);
  }
  registerMediaSessionHandlers();
  detachListeners = () => {
    for (const [type, listener] of listeners) {
      element.removeEventListener(type, listener);
    }
    if (audio === element) {
      audio = null;
    }
    detachListeners = null;
  };
  return detachListeners;
}

// Resolves once the element knows the media's duration, or false if it failed to load.
function whenMetadataLoaded(element: HTMLAudioElement): Promise<boolean> {
  if (element.readyState >= 1) {
    return Promise.resolve(true);
  }
  return new Promise((resolve) => {
    const done = (ok: boolean) => () => {
      element.removeEventListener("loadedmetadata", onLoaded);
      element.removeEventListener("error", onFailed);
      resolve(ok);
    };
    const onLoaded = done(true);
    const onFailed = done(false);
    element.addEventListener("loadedmetadata", onLoaded);
    element.addEventListener("error", onFailed);
  });
}

async function loadChapters(id: string, generation: number): Promise<void> {
  try {
    const response = await episodesApi.getChapters(id);
    if (generation === loadGeneration) {
      state.chapters = response.chapters ?? [];
    }
  } catch {
    if (generation === loadGeneration) {
      state.chapters = [];
    }
  }
}

async function loadTrack(index: number, options: { startSeconds?: number; autoplay?: boolean } = {}): Promise<void> {
  const item = state.queue[index];
  if (!item) {
    return;
  }
  loadGeneration += 1;
  const generation = loadGeneration;
  const start = options.startSeconds && options.startSeconds > 0 ? options.startSeconds : 0;
  state.index = index;
  state.currentTime = start;
  state.duration = item.Duration || 0;
  state.chapters = [];
  state.error = "";
  state.autoplayBlocked = false;
  lastAutoSkipStart = null;
  updateMediaMetadata();
  void loadChapters(item.ID, generation);
  if (!audio) {
    return;
  }
  audio.src = episodeAudioUrl(item.ID);
  audio.playbackRate = state.rate;
  audio.load();
  if (start > 0) {
    const loaded = await whenMetadataLoaded(audio);
    if (generation !== loadGeneration) {
      return;
    }
    if (loaded) {
      audio.currentTime = clampTime(start);
    }
  }
  if (options.autoplay !== false && generation === loadGeneration) {
    await play();
  }
}

// --- Commands ---------------------------------------------------------------------------

export async function play(): Promise<void> {
  if (!audio || !current.value) {
    return;
  }
  try {
    await audio.play();
    state.autoplayBlocked = false;
  } catch (error) {
    state.playing = false;
    // Browsers block audio that starts without a user gesture; the Play button works.
    if (error instanceof DOMException && error.name === "NotAllowedError") {
      state.autoplayBlocked = true;
    }
  }
}

export function pause(): void {
  audio?.pause();
}

export function toggle(): void {
  if (state.playing) {
    pause();
  } else {
    void play();
  }
}

export function seek(seconds: number): void {
  const target = clampTime(seconds);
  state.currentTime = target;
  if (audio) {
    audio.currentTime = target;
  }
  updatePositionState();
}

export function skip(deltaSeconds: number): void {
  seek(state.currentTime + deltaSeconds);
}

export async function next(): Promise<void> {
  if (hasNext.value) {
    await loadTrack(state.index + 1);
  }
}

export async function previous(): Promise<void> {
  if (state.currentTime > RESTART_THRESHOLD_SECONDS || !hasPrevious.value) {
    seek(0);
    return;
  }
  await loadTrack(state.index - 1);
}

export function setRate(rate: number): void {
  if (!(SPEED_OPTIONS as readonly number[]).includes(rate)) {
    return;
  }
  state.rate = rate;
  if (audio) {
    audio.playbackRate = rate;
  }
  try {
    window.localStorage.setItem(RATE_KEY, String(rate));
  } catch {
    // The rate still applies for this session.
  }
  updatePositionState();
}

export function skipSponsor(): void {
  const segment = currentSponsorSegment.value;
  if (segment) {
    seek(segment.end);
  }
}

export async function playQueue(items: PodcastItem[], startIndex = 0, startSeconds?: number): Promise<void> {
  if (items.length === 0) {
    return;
  }
  state.queue = [...items];
  await loadTrack(Math.min(Math.max(0, startIndex), items.length - 1), { startSeconds });
}

export async function playEpisode(item: PodcastItem, startSeconds?: number): Promise<void> {
  const queued = state.queue.findIndex((entry) => entry.ID === item.ID);
  if (queued >= 0) {
    // Already in the queue: jump to it instead of replacing the queue.
    state.queue[queued] = item;
    await loadTrack(queued, { startSeconds });
    return;
  }
  await playQueue([item], 0, startSeconds);
}

export function playEpisodeAt(item: PodcastItem, startSeconds: number): Promise<void> {
  if (current.value?.ID === item.ID && audio) {
    seek(startSeconds);
    return play();
  }
  return playEpisode(item, startSeconds);
}

export function enqueue(item: PodcastItem): void {
  if (!state.queue.some((entry) => entry.ID === item.ID)) {
    state.queue.push(item);
  }
}

export function playFromQueue(index: number): Promise<void> {
  return loadTrack(index);
}

export function removeFromQueue(index: number): void {
  if (index < 0 || index >= state.queue.length || index === state.index) {
    return;
  }
  state.queue.splice(index, 1);
  if (index < state.index) {
    state.index -= 1;
  }
}

export async function playEpisodeIds(ids: string[], startSeconds?: number): Promise<void> {
  state.loadingQueue = true;
  state.error = "";
  try {
    const loaded = await Promise.all(ids.map((id) => episodesApi.getById(id).catch(() => null)));
    const items = loaded.filter((item): item is PodcastItem => item !== null);
    if (items.length === 0) {
      state.error = "None of the requested episodes could be found.";
      return;
    }
    await playQueue(items, 0, startSeconds);
  } finally {
    state.loadingQueue = false;
  }
}

export async function playPodcast(podcastId: string): Promise<void> {
  state.loadingQueue = true;
  state.error = "";
  try {
    const items = sortNewestFirst(await podcastsApi.items(podcastId));
    if (items.length === 0) {
      state.error = "This podcast has no episodes to play yet.";
      return;
    }
    await playQueue(items);
  } catch (error) {
    state.error = getErrorMessage(error, "Could not load the podcast's episodes.");
  } finally {
    state.loadingQueue = false;
  }
}

export function stop(): void {
  loadGeneration += 1;
  audio?.pause();
  audio?.removeAttribute("src");
  audio?.load();
  state.queue = [];
  state.index = -1;
  state.currentTime = 0;
  state.duration = 0;
  state.chapters = [];
  state.playing = false;
  state.error = "";
  updateMediaMetadata();
}

// While something is queued the page reserves room for the player bar.
watch(
  () => state.queue.length > 0,
  (active) => {
    if (typeof document !== "undefined") {
      document.documentElement.classList.toggle("has-player", active);
    }
  },
);

export function usePlayer() {
  return {
    state,
    current,
    hasNext,
    hasPrevious,
    sponsorSegments,
    currentSponsorSegment,
    autoSkipEnabled,
    attach,
    play,
    pause,
    toggle,
    seek,
    skip,
    next,
    previous,
    setRate,
    skipSponsor,
    playQueue,
    playEpisode,
    playEpisodeAt,
    playEpisodeIds,
    playPodcast,
    playFromQueue,
    enqueue,
    removeFromQueue,
    stop,
  };
}
