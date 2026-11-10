// A jsdom <audio> with controllable media state, for player tests.
export interface FakeAudio extends HTMLAudioElement {
  playCalls: number;
  pauseCalls: number;
  rejectPlayWith: DOMException | null;
  setDuration(seconds: number): void;
  fire(type: string): void;
}

export function makeAudio(): FakeAudio {
  const audio = document.createElement("audio") as FakeAudio;
  let currentTime = 0;
  let duration = NaN;
  let readyState = 0;
  let paused = true;
  Object.defineProperties(audio, {
    currentTime: { get: () => currentTime, set: (v: number) => { currentTime = v; }, configurable: true },
    duration: { get: () => duration, configurable: true },
    readyState: { get: () => readyState, configurable: true },
    paused: { get: () => paused, configurable: true },
  });
  audio.playCalls = 0;
  audio.pauseCalls = 0;
  audio.rejectPlayWith = null;
  audio.play = () => {
    audio.playCalls += 1;
    if (audio.rejectPlayWith) {
      return Promise.reject(audio.rejectPlayWith);
    }
    paused = false;
    audio.dispatchEvent(new Event("play"));
    return Promise.resolve();
  };
  audio.pause = () => {
    audio.pauseCalls += 1;
    paused = true;
    audio.dispatchEvent(new Event("pause"));
  };
  audio.load = () => {
    readyState = 0;
  };
  audio.setDuration = (seconds: number) => {
    duration = seconds;
    readyState = 1;
    audio.dispatchEvent(new Event("loadedmetadata"));
  };
  audio.fire = (type: string) => audio.dispatchEvent(new Event(type));
  return audio;
}
