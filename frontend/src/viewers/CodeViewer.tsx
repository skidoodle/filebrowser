import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import { auth } from "../api/auth";
import { canWritePath } from "../lib/permissions";
import { hasEditToken } from "../lib/tokens";
import { useRoute } from "../lib/router";
import { EditorShell } from "./EditorShell";
import { ShikiDirect } from "./ShikiDirect";

const EXT_TO_LANG: Record<string, string> = {
  ts: "typescript", tsx: "tsx", mts: "typescript", cts: "typescript",
  js: "javascript", jsx: "jsx", mjs: "javascript", cjs: "javascript",
  json: "json", jsonc: "jsonc", har: "json", geojson: "json", topojson: "json",
  sh: "bash", bash: "bash", zsh: "bash", fish: "shell", ps1: "shell",
  css: "css", scss: "css", less: "css",
  html: "html", htm: "html", xml: "html", svg: "html", svelte: "html",
  kml: "html", gml: "html", gfs: "html", xsd: "html", plist: "html", resx: "html", rss: "html", atom: "html",
  py: "python", pyi: "python",
  yml: "yaml", yaml: "yaml",
  md: "markdown", markdown: "markdown",
  sql: "sql", graphql: "graphql", gql: "graphql",
  toml: "toml", hcl: "hcl", tf: "hcl", tfvars: "hcl",
  patch: "diff", diff: "diff",
  go: "go", rs: "rust", java: "java", kt: "kotlin", kts: "kotlin",
  c: "c", h: "c", cpp: "cpp", cc: "cpp", cxx: "cpp", hpp: "cpp",
  cs: "csharp", php: "php", phps: "php", rb: "ruby", swift: "swift", dart: "dart",
  lua: "lua", scala: "scala", ex: "elixir", exs: "elixir", erl: "erlang",
  hs: "haskell", clj: "clojure", r: "r", pl: "perl", pm: "perl",
  asm: "asm", zig: "zig", nim: "nim", sol: "solidity", vue: "vue",
};

const LANG_LABELS: Record<string, string> = {
  typescript: "TypeScript", javascript: "JavaScript", jsx: "JSX", tsx: "TSX",
  json: "JSON", jsonc: "JSONC", html: "HTML", css: "CSS",
  python: "Python", yaml: "YAML", markdown: "Markdown", graphql: "GraphQL",
  sql: "SQL", bash: "Bash", shell: "Shell", diff: "Diff", hcl: "HCL", toml: "TOML",
  go: "Go", rust: "Rust", java: "Java", kotlin: "Kotlin", c: "C", cpp: "C++",
  csharp: "C#", php: "PHP", ruby: "Ruby", swift: "Swift", dart: "Dart",
  lua: "Lua", scala: "Scala", elixir: "Elixir", erlang: "Erlang",
  haskell: "Haskell", clojure: "Clojure", r: "R", perl: "Perl",
  asm: "Assembly", zig: "Zig", nim: "Nim", solidity: "Solidity", vue: "Vue",
};

/** Files above this size skip Shiki highlighting and render as plain text. */
const PLAIN_TEXT_MAX = 2 * 1024 * 1024;

export function CodeViewer({
  path,
  extension,
  size,
}: {
  path: string;
  extension?: string;
  size?: number;
}) {
  const [code, setCode] = useState<string | null>(null);
  const route = useRoute();

  const [draft, setDraft] = useState<string | null>(
    route.page === "viewer" && route.edit ? "" : null,
  );
  const [error, setError] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const me = useQuery({ queryKey: ["me"], queryFn: auth.me, staleTime: 60_000 });
  const hasToken = hasEditToken(path);
  const canEdit = canWritePath(me.data, path) || hasToken || draft !== null;

  useEffect(() => {
    let cancelled = false;
    fetch(api.rawUrl(path, true))
      .then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.text();
      })
      .then((text) => {
        if (!cancelled) setCode(text);
      })
      .catch((e: Error) => {
        if (!cancelled) setError(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, [path]);

  const save = useMutation({
    mutationFn: async ({ exit = true }: { exit?: boolean } = {}) => {
      const content = draft ?? code ?? "";
      await api.save(path, content);
      return { content, exit };
    },
    onSuccess: ({ content, exit }) => {
      setCode(content);
      if (exit) {
        setDraft(null);
      } else {
        setDraft(content);
      }
      void queryClient.invalidateQueries({ queryKey: ["meta", path] });
      void queryClient.invalidateQueries({ queryKey: ["list"] });
    },
  });

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        if (canEdit && draft !== null && !save.isPending) {
          save.mutate({ exit: true });
        }
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [canEdit, draft, save]);

  if (error) return <p className="text-kumo-danger p-6">{error}</p>;
  if (code === null) return <p className="text-kumo-subtle p-6">loading…</p>;

  const ext = (extension ?? "").toLowerCase();
  const lang = EXT_TO_LANG[ext] ?? "";
  const isPlain = !lang || lang === "plaintext" || ext === "txt" || ext === "text" || ext === "log";
  const highlight = !isPlain && size !== undefined && size <= PLAIN_TEXT_MAX;

  const langLabel = !highlight
    ? "Plain text"
    : (LANG_LABELS[lang] ?? (lang ? lang.charAt(0).toUpperCase() + lang.slice(1) : "Text"));

  return (
    <EditorShell
      path={path}
      code={code}
      langLabel={langLabel}
      size={size}
      canEdit={canEdit}
      editing={draft !== null}
      draft={draft ?? code}
      saving={save.isPending}
      dirty={draft !== null && draft !== code}
      saveError={save.error instanceof Error ? save.error.message : null}
      onToggleEdit={() => setDraft(draft !== null ? null : code)}
      onSave={() => save.mutate({ exit: true })}
    >
      {draft !== null ? (
        <textarea
          autoFocus
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          spellCheck={false}
          className="text-kumo-default bg-kumo-base h-full min-h-0 w-full resize-none p-4 font-mono text-sm leading-relaxed outline-none"
        />
      ) : !highlight ? (
        <PlainText code={code} />
      ) : (
        <ShikiDirect code={code} lang={lang} />
      )}
    </EditorShell>
  );
}

/** Highlight-free rendering for plain text and huge files */
function PlainText({ code }: { code: string }) {
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
      <pre className="text-kumo-default min-w-0 flex-1 overflow-x-auto whitespace-pre font-mono text-sm leading-relaxed">
        {code}
      </pre>
    </div>
  );
}
