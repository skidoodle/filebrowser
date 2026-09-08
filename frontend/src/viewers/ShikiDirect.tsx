import { useEffect, useState } from "react";
import { usePrefs } from "../stores/prefs";

export function ShikiDirect({ code, lang }: { code: string; lang: string }) {
  const theme = usePrefs((s) => s.theme);
  const [html, setHtml] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const shikiTheme = theme === "dark" ? "vesper" : "github-light";
    void import("shiki")
      .then((shiki) => shiki.codeToHtml(code, { lang, theme: shikiTheme }))
      .then((out) => {
        if (!cancelled) setHtml(out);
      })
      .catch(() => {
        if (!cancelled) {
          setHtml(
            `<pre class="shiki-plain"><code>${code
              .replace(/&/g, "&amp;")
              .replace(/</g, "&lt;")
              .replace(/>/g, "&gt;")}</code></pre>`,
          );
        }
      });
    return () => {
      cancelled = true;
    };
  }, [code, lang, theme]);

  if (html === null) return <p className="text-kumo-subtle p-4">highlighting…</p>;

  return (
    <div
      className="text-kumo-default p-4 text-sm [&_pre]:bg-transparent! [&_pre]:p-0 [&_pre]:font-mono"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
