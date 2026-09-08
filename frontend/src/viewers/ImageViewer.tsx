import Lightbox, { ACTION_CLOSE, type Slide as LightboxSlide } from "yet-another-react-lightbox";
import Counter from "yet-another-react-lightbox/plugins/counter";
import Captions from "yet-another-react-lightbox/plugins/captions";
import Video from "yet-another-react-lightbox/plugins/video";
import Zoom from "yet-another-react-lightbox/plugins/zoom";
import "yet-another-react-lightbox/styles.css";
import "yet-another-react-lightbox/plugins/captions.css";
import "yet-another-react-lightbox/plugins/counter.css";
import { DownloadSimpleIcon } from "@phosphor-icons/react";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import { usePrefs } from "../stores/prefs";
import type { FileInfo } from "../types";

async function decodeTiff(url: string): Promise<string> {
  const UTIF = (await import("utif")).default;
  const buffer = await fetch(url).then((r) => r.arrayBuffer());
  const ifds = UTIF.decode(buffer);
  if (ifds.length === 0) throw new Error("no IFD in TIFF");
  UTIF.decodeImage(buffer, ifds[0]);
  const rgba = UTIF.toRGBA8(ifds[0]);
  const canvas = document.createElement("canvas");
  canvas.width = ifds[0].width;
  canvas.height = ifds[0].height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw Error("no 2d context");
  const imageData = ctx.createImageData(canvas.width, canvas.height);
  imageData.data.set(rgba);
  ctx.putImageData(imageData, 0, 0);
  return canvas.toDataURL("image/png");
}

/** Image formats <img> cannot render natively; decoded client-side instead. */
const NON_NATIVE_IMAGES = new Set(["tif", "tiff"]);

/** Formats the /api/big re-encode would break; streamed raw instead. */
const RAW_PREVIEW_EXTS = new Set(["gif", "svg"]);

function imageUrl(file: FileInfo): string {
  return RAW_PREVIEW_EXTS.has((file.extension ?? "").toLowerCase())
    ? api.rawUrl(file.path, true)
    : api.bigUrl(file.path);
}

const VIDEO_MIME: Record<string, string> = {
  mp4: "video/mp4",
  m4v: "video/mp4",
  webm: "video/webm",
  ogg: "video/ogg",
  ogv: "video/ogg",
  mov: "video/quicktime",
};

function toSlide(file: FileInfo): LightboxSlide {
  const ext = (file.extension ?? "").toLowerCase();
  if (file.type === "video" && ext in VIDEO_MIME) {
    return {
      type: "video",
      title: file.name,
      controls: true,
      preload: "metadata",
      sources: [{ src: api.rawUrl(file.path, true), type: file.mime ?? VIDEO_MIME[ext] }],
    };
  }
  return { src: imageUrl(file), title: file.name, alt: file.name };
}

export function ImageViewer({
  path,
  name,
  extension,
  onClose,
}: {
  path: string;
  name?: string;
  extension?: string;
  onClose: () => void;
}) {
  const prefs = usePrefs();
  const [error, setError] = useState<string | null>(null);
  const [tiffSrc, setTiffSrc] = useState<string | null>(null);
  const [active, setActive] = useState(0);

  const isTiff = NON_NATIVE_IMAGES.has((extension ?? "").toLowerCase());
  const dir = path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : ".";

  // Sibling media: the same listing (and cache key) the browser grid uses,
  // so opening from the grid is instant and follows the visible order.
  const siblings = useQuery({
    queryKey: ["list", dir, prefs.sortBy, prefs.sortOrder],
    queryFn: () => api.list(dir, prefs.sortBy, prefs.sortOrder),
    staleTime: 60_000,
  });

  // TIFF has no native browser support; decode to a PNG data URL first.
  useEffect(() => {
    if (!isTiff) return;
    let cancelled = false;
    decodeTiff(api.rawUrl(path, true))
      .then((s) => {
        if (!cancelled) setTiffSrc(s);
      })
      .catch((e: Error) => {
        if (!cancelled) setError(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, [isTiff, path]);

  const { slides, paths, index } = useMemo((): { slides: LightboxSlide[]; paths: string[]; index: number } => {
    if (isTiff) {
      return tiffSrc === null
        ? { slides: [], paths: [path], index: 0 }
        : { slides: [{ src: tiffSrc, title: name, alt: name }], paths: [path], index: 0 };
    }
    const media = (siblings.data?.items ?? []).filter((f) => {
      if (f.type === "image") return !NON_NATIVE_IMAGES.has((f.extension ?? "").toLowerCase());
      if (f.type === "video") return (f.extension ?? "").toLowerCase() in VIDEO_MIME;
      return false;
    });
    const at = media.findIndex((f) => f.path === path);
    if (at === -1) {
      // Listing unavailable or the file is not in it: view it on its own.
      const file = siblings.data?.items.find((f) => f.path === path);
      const src = file ? imageUrl(file) : api.bigUrl(path);
      return { slides: [{ src, title: name, alt: name }], paths: [path], index: 0 };
    }
    return { slides: media.map(toSlide), paths: media.map((f) => f.path), index: at };
  }, [isTiff, tiffSrc, name, path, siblings.data]);

  if (error) return <p className="text-kumo-danger p-6">Could not display this image: {error}</p>;
  if (slides.length === 0) return <p className="text-kumo-subtle p-6">loading…</p>;

  const hasVideo = slides.some((s) => s.type === "video");

  return (
    <Lightbox
      open
      close={onClose}
      slides={slides}
      index={index}
      carousel={{ finite: slides.length < 2 }}
      plugins={[Zoom, Captions, ...(hasVideo ? [Video] : []), ...(slides.length > 1 ? [Counter] : [])]}
      zoom={{ maxZoomPixelRatio: 4, scrollToZoom: true }}
      on={{ view: ({ index: i }) => setActive(i) }}
      toolbar={{
        buttons: [
          <a
            key="download"
            role="button"
            aria-label="Download"
            className="yarl__button"
            href={api.rawUrl(paths[Math.min(active, paths.length - 1)] ?? path)}
          >
            <DownloadSimpleIcon size={24} weight="fill" className="yarl__icon" />
          </a>,
          ACTION_CLOSE,
        ],
      }}
    />
  );
}
