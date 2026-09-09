import {
  ArrowLeftIcon,
  DownloadSimpleIcon,
  FileIcon,
  XIcon,
} from "@phosphor-icons/react";
import { Button, Loader } from "@cloudflare/kumo";
import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense, useEffect } from "react";
import { api } from "../api/client";
import { FileTypeIcon } from "../lib/icons";
import { navigate, useRoute } from "../lib/router";
import type { FileMeta } from "../types";

const CodeViewer = lazy(() => import("./CodeViewer").then((m) => ({ default: m.CodeViewer })));
const MarkdownPreview = lazy(() => import("./MarkdownPreview").then((m) => ({ default: m.MarkdownPreview })));
const PdfViewer = lazy(() => import("./PdfViewer").then((m) => ({ default: m.PdfViewer })));
const VideoPlayer = lazy(() => import("./VideoPlayer").then((m) => ({ default: m.VideoPlayer })));
const AudioPlayer = lazy(() => import("./AudioPlayer").then((m) => ({ default: m.AudioPlayer })));
const ImageViewer = lazy(() => import("./ImageViewer").then((m) => ({ default: m.ImageViewer })));

/**
 * Image extensions recognized before the meta request resolves, so the
 * lightbox mode can render without flashing the standard viewer chrome.
 */
const IMAGE_EXTS = new Set(["avif", "bmp", "gif", "ico", "jpeg", "jpg", "png", "svg", "tif", "tiff", "webp"]);

export function ViewerOverlay() {
  const route = useRoute();
  const view = route.page === "viewer" ? route.path : null;
  const meta = useMeta(view);
  const close = () => navigate({ page: "files", dir: route.dir }, { replace: true });

  useEffect(() => {
    if (!view) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view]);

  if (!view) return null;

  const file = meta.data;
  // Until meta answers, the extension decides whether the lightbox
  // or the standard viewer chrome is the right stage.
  const lightboxMode = file ? file.type === "image" : IMAGE_EXTS.has((view.split(".").pop() ?? "").toLowerCase());

  if (lightboxMode) {
    return (
      <div className="fixed inset-0 z-50 flex flex-col bg-black">
        {meta.isPending && <Loader className="mx-auto mt-16" />}
        {meta.isError && (
          <div className="flex flex-1 flex-col items-center justify-center gap-4">
            <p className="text-kumo-danger">{meta.error.message}</p>
            <Button variant="secondary" onClick={close}>
              Back
            </Button>
          </div>
        )}
        {file && <ViewerBody path={view} file={file} onClose={close} />}
      </div>
    );
  }

  return (
    <div className="bg-kumo-canvas fixed inset-0 z-50 flex flex-col">
      <header className="border-kumo-hairline flex h-14 shrink-0 items-center gap-2 border-b px-3">
        <Button variant="ghost" shape="square" aria-label="Back" icon={<ArrowLeftIcon size={20} />} onClick={close} />
        <div className="flex min-w-0 items-center gap-2.5">
          {file && <FileTypeIcon type={file.type} size={22} />}
          <p className="truncate leading-6 text-sm font-semibold">{file?.name ?? view}</p>
        </div>
        <div className="ml-auto flex items-center gap-1">
          <Button
            variant="ghost"
            shape="square"
            aria-label="Download"
            icon={<DownloadSimpleIcon size={20} weight="fill" />}
            onClick={() => window.location.assign(api.rawUrl(view))}
          />
          <Button variant="ghost" shape="square" aria-label="Close viewer" icon={<XIcon size={20} />} onClick={close} />
        </div>
      </header>

      <div className="flex min-h-0 flex-1 flex-col overflow-auto">
        {meta.isPending && <Loader className="mx-auto mt-16" />}
        {meta.isError && <p className="text-kumo-danger p-6">{meta.error.message}</p>}
        {file && <ViewerBody path={view} file={file} onClose={close} />}
      </div>
    </div>
  );
}

function useMeta(path: string | null) {
  return useQuery({
    queryKey: ["meta", path],
    queryFn: () => api.meta(path!),
    enabled: path !== null,
    staleTime: 60_000,
  });
}

function ViewerBody({ path, file, onClose }: { path: string; file: FileMeta; onClose: () => void }) {
  const ext = (file.extension ?? "").toLowerCase();

  const fallback = (
    <div className="flex flex-1 flex-col items-center justify-center gap-4">
      <FileIcon size={56} weight="fill" className="text-kumo-subtle" />
      <p className="text-kumo-subtle">No preview available for this format.</p>
      <Button variant="primary" icon={<DownloadSimpleIcon />} onClick={() => window.location.assign(api.rawUrl(path))}>
        Download {file.name}
      </Button>
    </div>
  );

  const body = (() => {
    switch (file.type) {
      case "text":
        if (ext === "md" || ext === "markdown" || ext === "mdx") {
          return <MarkdownPreview path={path} />;
        }
        return <CodeViewer key={path} path={path} extension={file.extension} size={file.size} />;
      case "pdf":
        return <PdfViewer path={path} />;
      case "video":
        return <VideoPlayer path={path} />;
      case "audio":
        return <AudioPlayer path={path} name={file.name} size={file.size} />;
      case "image":
        return <ImageViewer path={path} name={file.name} extension={file.extension} onClose={onClose} />;
      default:
        return fallback;
    }
  })();

  return <Suspense fallback={<Loader className="mx-auto mt-16" />}>{body}</Suspense>;
}
