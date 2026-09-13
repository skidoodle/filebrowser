import { GaugeIcon, SpeakerHighIcon, SpeakerLowIcon, SpeakerXIcon } from "@phosphor-icons/react";
import { Button, DropdownMenu, Tooltip } from "@cloudflare/kumo";
import type { CSSProperties, ReactNode } from "react";
import { MenuItemRow } from "../components/MenuItemRow";

const PLAYBACK_RATES = [0.5, 0.75, 1, 1.25, 1.5, 2] as const;

/** Formats seconds as M:SS or H:MM:SS. */
function formatTime(s: number): string {
  if (!Number.isFinite(s) || s < 0) return "0:00";
  const total = Math.floor(s);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const sec = String(total % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${sec}` : `${m}:${sec}`;
}

interface MediaSliderProps {
  value: number;
  max: number;
  buffered?: number;
  onSeek: (value: number) => void;
  label: string;
  className?: string;
}

export function MediaSlider({ value, max, buffered = 0, onSeek, label, className = "" }: MediaSliderProps) {
  const pct = (v: number) => `${max > 0 ? Math.min(100, (v / max) * 100) : 0}%`;
  const style = {
    "--media-fill": pct(value),
    "--media-buffer": pct(Math.max(buffered, value)),
  } as CSSProperties;

  return (
    <input
      type="range"
      className={`media-slider ${className}`}
      min={0}
      max={max || 0}
      step="any"
      value={Math.min(value, max || 0)}
      aria-label={label}
      aria-valuetext={label === "Seek" ? formatTime(value) : undefined}
      onChange={(e) => onSeek(Number(e.target.value))}
      style={style}
      draggable={false}
    />
  );
}

export function MediaTime({ current, total, className = "" }: { current: number; total: number; className?: string }) {
  return (
    <span className={`font-mono text-xs tabular-nums whitespace-nowrap ${className}`}>
      {formatTime(current)} <span className="opacity-60">/ {formatTime(total)}</span>
    </span>
  );
}

function VolumeIcon({ volume, muted }: { volume: number; muted: boolean }) {
  if (muted || volume === 0) return <SpeakerXIcon size={18} weight="fill" />;
  if (volume < 0.5) return <SpeakerLowIcon size={18} weight="fill" />;
  return <SpeakerHighIcon size={18} weight="fill" />;
}

interface VolumeControlProps {
  volume: number;
  muted: boolean;
  onToggleMute: () => void;
  onVolume: (v: number) => void;
  dark?: boolean;
  sliderClassName?: string;
}

export function VolumeControl({ volume, muted, onToggleMute, onVolume, dark = false, sliderClassName = "" }: VolumeControlProps) {
  const level = muted ? 0 : volume;
  return (
    <div className="group/vol flex items-center">
      <Tooltip
        content={muted ? "Unmute (m)" : "Mute (m)"}
        side="top"
        render={
          <Button
            variant="ghost"
            shape="square"
            size="sm"
            aria-label={muted ? "Unmute" : "Mute"}
            icon={<VolumeIcon volume={volume} muted={muted} />}
            className={dark ? onVideo() : undefined}
            onClick={onToggleMute}
          />
        }
      />
      <MediaSlider
        value={level}
        max={1}
        onSeek={onVolume}
        label="Volume"
        className={`w-0 opacity-0 transition-[width,opacity] duration-200 group-hover/vol:w-20 group-hover/vol:opacity-100 group-focus-within/vol:w-20 group-focus-within/vol:opacity-100 max-sm:hidden ${sliderClassName}`}
      />
    </div>
  );
}

interface RateMenuProps {
  rate: number;
  onRate: (rate: number) => void;
  dark?: boolean;
}

export function RateMenu({ rate, onRate, dark = false }: RateMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenu.Trigger
        render={(p) => (
          <Button
            {...p}
            variant="ghost"
            shape="square"
            size="sm"
            aria-label={`Playback speed (currently ${rate === 1 ? "normal" : `${rate}×`})`}
            icon={<GaugeIcon size={18} />}
            className={dark ? onVideo() : undefined}
          />
        )}
      />
      <DropdownMenu.Content align="end" className="min-w-28 z-60">
        {PLAYBACK_RATES.map((r) => (
          <MenuItemRow
            key={r}
            label={r === 1 ? "Normal" : `${r}×`}
            active={rate === r}
            onClick={() => onRate(r)}
          />
        ))}
      </DropdownMenu.Content>
    </DropdownMenu>
  );
}

const onVideo = (...parts: (string | false | undefined)[]): string =>
  ["text-white/90", "hover:bg-white/15!", "focus-visible:ring-white/60!", ...parts].filter(Boolean).join(" ");

export function MediaButton({
  tooltip,
  onClick,
  icon,
  ariaLabel,
  dark = false,
}: {
  tooltip: string;
  onClick: () => void;
  icon: ReactNode;
  ariaLabel?: string;
  dark?: boolean;
}) {
  return (
    <Tooltip
      content={tooltip}
      side="top"
      render={
        <Button
          variant="ghost"
          shape="square"
          size="sm"
          aria-label={ariaLabel ?? tooltip}
          icon={icon}
          className={dark ? onVideo() : undefined}
          onClick={onClick}
        />
      }
    />
  );
}
