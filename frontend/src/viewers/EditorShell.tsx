import {
  CheckIcon,
  CopyIcon,
  DownloadSimpleIcon,
  FloppyDiskIcon,
  NotePencilIcon,
  XIcon,
} from "@phosphor-icons/react";
import { Button, Tooltip } from "@cloudflare/kumo";
import { useState, type ReactNode } from "react";
import { api } from "../api/client";
import { formatBytes } from "../lib/format";

function CopyButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);

  const copy = () => {
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  return (
    <Tooltip
      content={copied ? "Copied" : label}
      render={
        <Button
          variant="ghost"
          shape="square"
          size="sm"
          aria-label={label}
          icon={copied ? <CheckIcon size={16} weight="bold" className="text-kumo-success" /> : <CopyIcon size={16} />}
          onClick={copy}
        />
      }
    />
  );
}

interface EditorShellProps {
  path: string;
  code: string;
  langLabel: string;
  size?: number;
  canEdit?: boolean;
  editing?: boolean;
  draft?: string;
  saving?: boolean;
  dirty?: boolean;
  saveError?: string | null;
  onToggleEdit?: () => void;
  onSave?: () => void;
  children: ReactNode;
}

export function EditorShell({
  path,
  code,
  langLabel,
  size,
  canEdit = false,
  editing = false,
  draft,
  saving = false,
  dirty = false,
  saveError = null,
  onToggleEdit,
  onSave,
  children,
}: EditorShellProps) {
  const lines = (editing && draft !== undefined ? draft : code).split("\n").length;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-kumo-hairline bg-kumo-canvas/60 flex h-9 shrink-0 items-center gap-2 border-b px-3">
        <span className="text-kumo-default truncate text-xs font-semibold">{langLabel}</span>
        <span className="text-kumo-subtle text-xs">{lines.toLocaleString()} lines</span>
        {size !== undefined && <span className="text-kumo-subtle text-xs">{formatBytes(size)}</span>}
        {dirty && <span className="text-kumo-warning text-xs">unsaved</span>}
        {saveError && <span className="text-kumo-danger truncate text-xs">{saveError}</span>}
        <div className="ml-auto flex items-center gap-0.5">
          {canEdit && editing && (
            <>
              <Tooltip
                content="Discard changes"
                render={
                  <Button
                    variant="ghost"
                    shape="square"
                    size="sm"
                    aria-label="Discard changes"
                    icon={<XIcon size={16} />}
                    onClick={onToggleEdit}
                  />
                }
              />
              <Tooltip
                content="Save"
                render={
                  <Button
                    variant="primary"
                    shape="square"
                    size="sm"
                    aria-label="Save"
                    loading={saving}
                    disabled={!dirty}
                    icon={<FloppyDiskIcon size={16} weight="fill" />}
                    onClick={onSave}
                  />
                }
              />
            </>
          )}
          {canEdit && !editing && (
            <Tooltip
              content="Edit"
              render={
                <Button
                  variant="ghost"
                  shape="square"
                  size="sm"
                  aria-label="Edit"
                  icon={<NotePencilIcon size={16} />}
                  onClick={onToggleEdit}
                />
              }
            />
          )}
          {!editing && <CopyButton text={code} />}
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
      </div>
      <div className="bg-kumo-base min-h-0 flex-1 overflow-auto">{children}</div>
    </div>
  );
}
