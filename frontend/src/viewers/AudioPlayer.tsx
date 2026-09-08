import { DownloadSimpleIcon, PauseIcon, PlayIcon } from "@phosphor-icons/react";
import { Button, Tooltip } from "@cloudflare/kumo";
import { useCallback, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { api } from "../api/client";
import { formatBytes } from "../lib/format";
import { useMediaPlayer } from "../lib/useMediaPlayer";
import { MediaButton, MediaSlider, MediaTime, RateMenu, VolumeControl } from "./MediaControls";

export function AudioPlayer({
  path,
  name,
  size,
}: {
  path: string;
  name: string;
  size?: number;
}) {
  const { mediaRef, state, actions } = useMediaPlayer<HTMLAudioElement>();

  const onKeyDown = useCallback((e: ReactKeyboardEvent) => actions.handleMediaKey(e), [actions]);

  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-4 sm:p-6">
      <div
        role="region"
        aria-label="Audio player"
        tabIndex={0}
        onKeyDown={onKeyDown}
        className="bg-kumo-base ring-kumo-line w-full max-w-xl rounded-2xl p-4 ring-1 outline-none focus-visible:ring-2 focus-visible:ring-kumo-brand sm:p-5"
      >
        <div className="flex items-start gap-2">
          <div className="min-w-0 flex-1">
            <p className="truncate leading-7 font-semibold" title={name}>
              {name}
            </p>
            {size !== undefined && <p className="text-kumo-subtle text-xs">{formatBytes(size)}</p>}
          </div>
          <Tooltip
            content="Download"
            render={
              <Button
                variant="ghost"
                shape="square"
                size="sm"
                aria-label="Download"
                icon={<DownloadSimpleIcon size={16} />}
                onClick={() => window.location.assign(api.rawUrl(path))}
              />
            }
          />
        </div>

        <div className="mt-4 flex items-center gap-2 sm:gap-3">
          <MediaButton
            tooltip={state.playing ? "Pause (k)" : "Play (k)"}
            onClick={actions.togglePlay}
            icon={
              state.playing ? (
                <PauseIcon size={18} weight="fill" />
              ) : (
                <PlayIcon size={18} weight="fill" />
              )
            }
          />
          <MediaTime current={state.time} total={state.duration} className="text-kumo-subtle hidden min-[420px]:block" />
          <MediaSlider
            value={state.time}
            max={state.duration}
            buffered={state.buffered}
            onSeek={actions.seekTo}
            label="Seek"
            className="media-slider-light min-w-0 flex-1"
          />
          <RateMenu rate={state.rate} onRate={actions.setRate} />
          <VolumeControl
            volume={state.volume}
            muted={state.muted}
            onToggleMute={actions.toggleMute}
            onVolume={actions.setVolume}
            sliderClassName="media-slider-light"
          />
        </div>

        {state.error && (
          <p className="text-kumo-danger mt-3 text-xs">This format can't be played in the browser — use download instead.</p>
        )}

        <audio ref={mediaRef} src={api.rawUrl(path, true)} preload="metadata" className="hidden" />
      </div>
    </div>
  );
}
