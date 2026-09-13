import {
  DownloadSimpleIcon,
  PauseIcon,
  PlayIcon,
  SubtitlesIcon,
  SubtitlesSlashIcon,
  WarningCircleIcon,
} from "@phosphor-icons/react";
import { Button, Loader } from "@cloudflare/kumo";
import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { api } from "../api/client";
import { useMediaPlayer } from "../lib/useMediaPlayer";
import { MediaButton, MediaSlider, MediaTime, RateMenu, VolumeControl } from "./MediaControls";

const HIDE_DELAY_MS = 2500;

function useAutoHideControls(playing: boolean, delayMs = 2500) {
  const [uiVisible, setUiVisible] = useState(true);
  const hideTimer = useRef<number | undefined>(undefined);
  const playingRef = useRef(playing);

  const wake = useCallback(() => {
    setUiVisible(true);
    window.clearTimeout(hideTimer.current);
    hideTimer.current = window.setTimeout(() => {
      if (playingRef.current) setUiVisible(false);
    }, delayMs);
  }, [delayMs]);

  useEffect(() => {
    playingRef.current = playing;
  }, [playing]);

  useEffect(() => {
    if (!playing) {
      window.clearTimeout(hideTimer.current);
      return;
    }
    hideTimer.current = window.setTimeout(() => setUiVisible(false), delayMs);
    return () => window.clearTimeout(hideTimer.current);
  }, [playing, delayMs]);

  useEffect(() => () => window.clearTimeout(hideTimer.current), []);

  return { uiVisible, wake };
}

type MediaPlayerActions = ReturnType<typeof useMediaPlayer<HTMLVideoElement>>["actions"];

function VideoControlsBar({
  state,
  actions,
  pipSupported,
  fullscreenSupported,
}: {
  state: ReturnType<typeof useMediaPlayer<HTMLVideoElement>>["state"];
  actions: MediaPlayerActions;
  pipSupported: boolean;
  fullscreenSupported: boolean;
}) {
  return (
    <div className="absolute inset-x-0 bottom-0 bg-linear-to-t from-black/80 via-black/40 to-transparent px-2 pt-8 pb-[max(0.5rem,env(safe-area-inset-bottom))] sm:px-3">
      <MediaSlider
        value={state.time}
        max={state.duration}
        buffered={state.buffered}
        onSeek={actions.seekTo}
        label="Seek"
        className="w-full"
      />
      <div className="mt-1 flex items-center gap-0.5 sm:gap-1">
        <MediaButton
          tooltip={state.playing ? "Pause (k)" : "Play (k)"}
          onClick={actions.togglePlay}
          dark
          icon={state.playing ? <PauseIcon size={18} weight="fill" /> : <PlayIcon size={18} weight="fill" />}
        />
        <MediaTime current={state.time} total={state.duration} className="text-white/90" />

        <div className="ml-auto flex items-center gap-0.5 sm:gap-1">
          <MediaButton
            tooltip={state.captionsOn ? "Hide subtitles (c)" : "Show subtitles (c)"}
            onClick={actions.toggleCaptions}
            dark
            icon={state.captionsOn ? <SubtitlesIcon size={18} weight="fill" /> : <SubtitlesSlashIcon size={18} />}
          />
          <RateMenu rate={state.rate} onRate={actions.setRate} dark />
          {pipSupported && (
            <MediaButton
              tooltip={state.pip ? "Leave picture-in-picture" : "Picture-in-picture"}
              onClick={actions.togglePip}
              dark
              icon={
                <svg viewBox="0 0 24 24" className="size-4 fill-current" aria-hidden>
                  <path d="M3 5.5A1.5 1.5 0 0 1 4.5 4h15A1.5 1.5 0 0 1 21 5.5v13a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 18.5zm2 .5v11h9v-4.5a1 1 0 0 1 1-1h4V6z" />
                </svg>
              }
            />
          )}
          <VolumeControl
            volume={state.volume}
            muted={state.muted}
            onToggleMute={actions.toggleMute}
            onVolume={actions.setVolume}
            dark
          />
          {fullscreenSupported && (
            <MediaButton
              tooltip={state.fullscreen ? "Exit fullscreen (f)" : "Fullscreen (f)"}
              onClick={actions.toggleFullscreen}
              dark
              icon={
                state.fullscreen ? (
                  <svg viewBox="0 0 24 24" className="size-4 fill-current" aria-hidden>
                    <path d="M9 3a1 1 0 0 1 1 1v4a1 1 0 0 1-1 1H5a1 1 0 0 1 0-2h3V4a1 1 0 0 1 1-1m6 0a1 1 0 0 1 1 1v3h3a1 1 0 1 1 0 2h-4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1M5 15h4a1 1 0 0 1 1 1v4a1 1 0 1 1-2 0v-3H5a1 1 0 1 1 0-2m10 0h4a1 1 0 1 1 0 2h-3v3a1 1 0 1 1-2 0v-4a1 1 0 0 1 1-1" />
                  </svg>
                ) : (
                  <svg viewBox="0 0 24 24" className="size-4 fill-current" aria-hidden>
                    <path d="M4 4a1 1 0 0 1 1-1h5a1 1 0 1 1 0 2H6v4a1 1 0 0 1-2 0zm10 0a1 1 0 0 1 1-1h5v5a1 1 0 1 1-2 0V6h-3a1 1 0 0 1-1-1M4 15a1 1 0 0 1 2 0v3h4a1 1 0 1 1 0 2H5zm11 0a1 1 0 0 1 1 1v3h-1a1 1 0 1 1-2 0v-4z" />
                  </svg>
                )
              }
            />
          )}
        </div>
      </div>
    </div>
  );
}

export function VideoPlayer({ path }: { path: string }) {
  const { mediaRef, surfaceRef, state, actions, pipSupported } = useMediaPlayer<HTMLVideoElement>();
  const { uiVisible, wake } = useAutoHideControls(state.playing, HIDE_DELAY_MS);

  const onKeyDown = useCallback(
    (e: ReactKeyboardEvent) => {
      if (actions.handleMediaKey(e)) {
        wake();
        return;
      }
      if (e.key === "f" || e.key === "F") {
        e.preventDefault();
        actions.toggleFullscreen();
        wake();
        return;
      }
      if (e.key === "c" || e.key === "C") {
        e.preventDefault();
        actions.toggleCaptions();
        wake();
      }
    },
    [actions, wake],
  );

  const fullscreenSupported = typeof document !== "undefined" && document.fullscreenEnabled !== false;
  const showChrome = !state.error && (!state.playing || uiVisible);

  return (
    <div
      ref={surfaceRef}
      role="region"
      aria-label="Video player"
      tabIndex={0}
      className={`bg-black relative flex min-h-0 flex-1 items-center justify-center outline-none select-none ${state.playing && !uiVisible ? "cursor-none" : ""
        }`}
      onPointerMove={wake}
      onPointerDown={wake}
      onKeyDown={onKeyDown}
    >
      <video
        ref={mediaRef}
        src={api.rawUrl(path, true)}
        preload="metadata"
        playsInline
        className="max-h-full max-w-full outline-none md:rounded-xl"
        onClick={actions.togglePlay}
        onDoubleClick={actions.toggleFullscreen}
      />

      {!state.error && (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
          {!state.playing && !state.waiting && !state.ended && (
            <button
              type="button"
              aria-label="Play"
              className="pointer-events-auto flex size-18 items-center justify-center rounded-full bg-white/15 text-white ring-3 ring-white/30 backdrop-blur-sm transition-transform duration-150 ease-out hover:scale-105 hover:bg-white/25 active:scale-97"
              onClick={actions.togglePlay}
            >
              <PlayIcon size={30} weight="fill" className="ml-1" />
            </button>
          )}
          {state.waiting && <Loader className="pointer-events-auto" />}
        </div>
      )}

      {state.error && (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black/80 p-6 text-center">
          <WarningCircleIcon size={40} weight="fill" className="text-kumo-danger" />
          <p className="text-sm text-white/90">
            This format can't be played in the browser — the codec is probably missing.
          </p>
          <Button
            variant="secondary"
            icon={<DownloadSimpleIcon />}
            onClick={() => window.location.assign(api.rawUrl(path))}
          >
            Download instead
          </Button>
        </div>
      )}

      {showChrome && (
        <VideoControlsBar
          state={state}
          actions={actions}
          pipSupported={pipSupported}
          fullscreenSupported={fullscreenSupported}
        />
      )}
    </div>
  );
}
