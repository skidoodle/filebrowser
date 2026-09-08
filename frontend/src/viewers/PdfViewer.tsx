import { useEffect, useRef, useState } from "react";
import * as pdfjs from "pdfjs-dist";
import workerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import { api } from "../api/client";
import { withBasePath } from "../lib/base";

pdfjs.GlobalWorkerOptions.workerSrc = withBasePath(workerUrl);

export function PdfViewer({ path }: { path: string }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [error, setError] = useState(false);
  const [pages, setPages] = useState<{ total: number; done: number } | null>(null);

  useEffect(() => {
    let cancelled = false;
    const task = pdfjs.getDocument({ url: api.rawUrl(path, true) });

    task.promise
      .then(async (doc) => {
        const container = containerRef.current;
        if (!container || cancelled) return;
        container.replaceChildren();
        setPages({ total: doc.numPages, done: 0 });

        for (let i = 1; i <= doc.numPages; i++) {
          if (cancelled) return;
          const page = await doc.getPage(i);
          const base = page.getViewport({ scale: 1 });
          const scale = Math.min(1.5, Math.max(0.75, (container.clientWidth - 48) / base.width));
          const viewport = page.getViewport({ scale });
          const canvas = document.createElement("canvas");
          canvas.width = viewport.width;
          canvas.height = viewport.height;
          canvas.className = "mx-auto my-4 rounded-lg bg-white shadow-lg";
          container.appendChild(canvas);
          const ctx = canvas.getContext("2d");
          if (!ctx) return;
          await page.render({ canvas, canvasContext: ctx, viewport }).promise;
          if (!cancelled) setPages({ total: doc.numPages, done: i });
        }
      })
      .catch(() => {
        if (!cancelled) setError(true);
      });

    return () => {
      cancelled = true;
      void task.destroy();
    };
  }, [path]);

  if (error) {
    return <p className="text-kumo-danger p-6">This PDF could not be rendered — use download instead.</p>;
  }

  return (
    <div className="flex h-full flex-col">
      {pages && (
        <p className="text-kumo-subtle px-6 py-2 text-xs">
          page {pages.done} / {pages.total}
        </p>
      )}
      <div ref={containerRef} className="flex-1 overflow-auto px-4 pb-8" />
    </div>
  );
}
