import DOMPurify from "dompurify";
import { marked } from "marked";
import { useEffect, useState } from "react";
import { api } from "../api/client";

export function MarkdownPreview({ path }: { path: string }) {
  const [md, setMd] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch(api.rawUrl(path, true))
      .then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.text();
      })
      .then((text) => {
        if (!cancelled) setMd(text);
      })
      .catch((e: Error) => {
        if (!cancelled) setError(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, [path]);

  if (error) return <p className="text-kumo-danger p-6">{error}</p>;
  if (md === null) return <p className="text-kumo-subtle p-6">loading…</p>;

  const html = DOMPurify.sanitize(marked.parse(md, { async: false }));

  return (
    <div
      className="markdown-body mx-auto max-w-3xl p-8 [&_a]:text-kumo-link [&_blockquote]:border-kumo-line [&_blockquote]:border-l-4 [&_blockquote]:pl-4 [&_code]:bg-kumo-tint [&_code]:rounded [&_code]:px-1 [&_h1]:mb-4 [&_h1]:text-2xl [&_h1]:font-semibold [&_h2]:mb-3 [&_h2]:mt-6 [&_h2]:text-xl [&_h2]:font-semibold [&_h3]:mb-2 [&_h3]:mt-4 [&_h3]:font-semibold [&_img]:max-w-full [&_li]:ml-6 [&_li]:list-disc [&_p]:my-3 [&_pre]:bg-kumo-tint [&_pre]:my-4 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:p-4 [&_table]:w-full"
      // Sanitized above; guests can only ever inject markup into their own session.
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
