import { useEffect, useState } from "react";
import { useEffectiveTheme } from "../stores/prefs";

export function ShikiDirect({ code, lang }: { code: string; lang: string }) {
  const effectiveTheme = useEffectiveTheme();
  const [html, setHtml] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const shikiTheme = effectiveTheme === "dark" ? "github-dark" : "github-light";
    void import("shiki")
      .then((shiki) => shiki.codeToHtml(code, { lang, theme: shikiTheme }))
      .then((out) => {
        if (!cancelled) setHtml(out);
      })
      .catch((err: unknown) => {
        console.warn("Shiki highlight error:", err);
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
  }, [code, lang, effectiveTheme]);

  if (html === null) return <p className="text-kumo-subtle p-4 font-mono text-sm">loading…</p>;

  const lines = code.split("\n");
  const isSingleLine = lines.length <= 1;

  return (
    <div className="flex p-4 font-mono text-sm leading-relaxed">
      {!isSingleLine && (
        <div className="text-kumo-subtle select-none pr-4 text-right opacity-40">
          {lines.map((_, i) => (
            <div key={i} className="leading-relaxed">
              {i + 1}
            </div>
          ))}
        </div>
      )}
      <div
        className="text-kumo-default min-w-0 flex-1 overflow-x-auto [&_pre]:bg-transparent! [&_pre]:p-0! [&_pre]:m-0! [&_pre]:font-mono [&_pre]:text-sm [&_pre]:leading-relaxed! [&_code]:font-mono [&_code]:text-sm [&_code]:leading-relaxed!"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}
