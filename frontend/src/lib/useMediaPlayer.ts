import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";

export interface MediaState {
  playing: boolean;
  waiting: boolean;
  ended: boolean;
  error: boolean;
  duration: number;
  time: number;
  buffered: number;
  volume: number;
  muted: boolean;
  rate: number;
  fullscreen: boolean;
  pip: boolean;
  captionsOn: boolean;
}

const INITIAL: MediaState = {
  playing: false,
  waiting: false,
  ended: false,
  error: false,
  duration: 0,
  time: 0,
  buffered: 0,
  volume: 1,
  muted: false,
  rate: 1,
  fullscreen: false,
  pip: false,
  captionsOn: false,
};

const clamp01 = (v: number) => Math.min(1, Math.max(0, v));

interface VideoWithWebkitFullscreen extends HTMLVideoElement {
  webkitEnterFullscreen?: () => void;
}

export function useMediaPlayer<T extends HTMLMediaElement = HTMLMediaElement>() {
  const mediaRef = useRef<T | null>(null);
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const [state, setState] = useState<MediaState>(INITIAL);
  const patch = useCallback((p: Partial<MediaState>) => setState((s) => ({ ...s, ...p })), []);

  useEffect(() => {
    const el = mediaRef.current;
    if (!el) return;

    const readBuffered = () => {
      for (let i = 0; i < el.buffered.length; i++) {
        if (el.buffered.start(i) <= el.currentTime && el.currentTime <= el.buffered.end(i)) {
          return el.buffered.end(i);
        }
      }
      return 0;
    };
    const onTime = () => patch({ time: el.currentTime, buffered: readBuffered() });
    const onMeta = () => patch({ duration: Number.isFinite(el.duration) ? el.duration : 0 });
    const onProgress = () => patch({ buffered: readBuffered() });
    const onPlay = () => patch({ playing: true, ended: false, error: false });
    const onPause = () => patch({ playing: false });
    const onWaiting = () => patch({ waiting: true });
    const onPlaying = () => patch({ waiting: false });
    const onEnded = () => patch({ playing: false, ended: true });
    const onError = () => patch({ error: true, playing: false, waiting: false });
    const onVolume = () => patch({ volume: el.volume, muted: el.muted });
    const onRate = () => patch({ rate: el.playbackRate });
    const onFullscreen = () => patch({ fullscreen: document.fullscreenElement === surfaceRef.current });
    const onPip = () => patch({ pip: document.pictureInPictureElement === el });

    el.addEventListener("timeupdate", onTime);
    el.addEventListener("durationchange", onMeta);
    el.addEventListener("loadedmetadata", onMeta);
    el.addEventListener("progress", onProgress);
    el.addEventListener("play", onPlay);
    el.addEventListener("pause", onPause);
    el.addEventListener("waiting", onWaiting);
    el.addEventListener("playing", onPlaying);
    el.addEventListener("canplay", onPlaying);
    el.addEventListener("ended", onEnded);
    el.addEventListener("error", onError);
    el.addEventListener("volumechange", onVolume);
    el.addEventListener("ratechange", onRate);
    el.addEventListener("enterpictureinpicture", onPip);
    el.addEventListener("leavepictureinpicture", onPip);
    document.addEventListener("fullscreenchange", onFullscreen);

    patch({
      volume: el.volume,
      muted: el.muted,
      duration: Number.isFinite(el.duration) ? el.duration : 0,
    });

    return () => {
      el.removeEventListener("timeupdate", onTime);
      el.removeEventListener("durationchange", onMeta);
      el.removeEventListener("loadedmetadata", onMeta);
      el.removeEventListener("progress", onProgress);
      el.removeEventListener("play", onPlay);
      el.removeEventListener("pause", onPause);
      el.removeEventListener("waiting", onWaiting);
      el.removeEventListener("playing", onPlaying);
      el.removeEventListener("canplay", onPlaying);
      el.removeEventListener("ended", onEnded);
      el.removeEventListener("error", onError);
      el.removeEventListener("volumechange", onVolume);
      el.removeEventListener("ratechange", onRate);
      el.removeEventListener("enterpictureinpicture", onPip);
      el.removeEventListener("leavepictureinpicture", onPip);
      document.removeEventListener("fullscreenchange", onFullscreen);
    };
  }, [patch]);

  const togglePlay = useCallback(() => {
    const el = mediaRef.current;
    if (!el) return;
    if (el.paused) {
      void el.play().catch(() => { });
    } else {
      el.pause();
    }
  }, []);

  const seekTo = useCallback((t: number) => {
    const el = mediaRef.current;
    if (!el) return;
    const max = Number.isFinite(el.duration) ? el.duration : Number.MAX_SAFE_INTEGER;
    el.currentTime = Math.min(max, Math.max(0, t));
  }, []);

  const seekBy = useCallback(
    (delta: number) => {
      const el = mediaRef.current;
      if (el) seekTo(el.currentTime + delta);
    },
    [seekTo],
  );

  const setVolume = useCallback((v: number) => {
    const el = mediaRef.current;
    if (!el) return;
    el.volume = clamp01(v);
    if (v > 0) el.muted = false;
  }, []);

  const toggleMute = useCallback(() => {
    const el = mediaRef.current;
    if (el) el.muted = !el.muted;
  }, []);

  const setRate = useCallback((rate: number) => {
    const el = mediaRef.current;
    if (el) el.playbackRate = rate;
  }, []);

  const toggleFullscreen = useCallback(() => {
    const surface = surfaceRef.current;
    const el = mediaRef.current as VideoWithWebkitFullscreen | null;
    if (document.fullscreenElement) {
      void document.exitFullscreen();
    } else if (surface?.requestFullscreen) {
      void surface.requestFullscreen().catch(() => { });
    } else if (el?.webkitEnterFullscreen) {
      el.webkitEnterFullscreen();
    }
  }, []);

  const pipSupported = typeof document !== "undefined" && document.pictureInPictureEnabled === true;

  const togglePip = useCallback(() => {
    const el = mediaRef.current;
    if (!el) return;
    if (document.pictureInPictureElement) {
      void document.exitPictureInPicture();
    } else {
      void (el as unknown as HTMLVideoElement).requestPictureInPicture().catch(() => { });
    }
  }, []);

  const toggleCaptions = useCallback(() => {
    const el = mediaRef.current;
    const track = el?.textTracks[0];
    if (!track) return;
    const next = track.mode === "showing" ? "hidden" : "showing";
    // eslint-disable-next-line react-hooks/immutability
    track.mode = next;
    patch({ captionsOn: next === "showing" });
  }, [mediaRef, patch]);

  const handleMediaKey = useCallback(
    (e: ReactKeyboardEvent): boolean => {
      const el = mediaRef.current;
      if (!el) return false;
      switch (e.key) {
        case " ":
        case "k":
        case "K":
          e.preventDefault();
          togglePlay();
          return true;
        case "ArrowLeft":
          e.preventDefault();
          seekBy(-5);
          return true;
        case "ArrowRight":
          e.preventDefault();
          seekBy(5);
          return true;
        case "j":
        case "J":
          e.preventDefault();
          seekBy(-10);
          return true;
        case "l":
        case "L":
          e.preventDefault();
          seekBy(10);
          return true;
        case "ArrowUp":
          e.preventDefault();
          setVolume(el.volume + 0.1);
          return true;
        case "ArrowDown":
          e.preventDefault();
          setVolume(el.volume - 0.1);
          return true;
        case "m":
        case "M":
          e.preventDefault();
          toggleMute();
          return true;
        default:
          return false;
      }
    },
    [seekBy, setVolume, toggleMute, togglePlay],
  );

  return {
    mediaRef,
    surfaceRef,
    state,
    actions: {
      togglePlay,
      seekTo,
      seekBy,
      setVolume,
      toggleMute,
      setRate,
      toggleFullscreen,
      togglePip,
      toggleCaptions,
      handleMediaKey,
    },
    pipSupported,
  };
}
