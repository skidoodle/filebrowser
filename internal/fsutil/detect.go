package fsutil

import (
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/skidoodle/filebrowser/internal/storage"
)

type kind struct {
	typ storage.FileType
}

// extKinds maps file extensions (without dot, lowercase) to types.
var extKinds = map[string]kind{}

// groups lists extensions per type; mime type per extension is resolved
// via the mime package plus explicit overrides for formats operating
// systems and browsers commonly miss.
var groups = map[storage.FileType][]string{
	storage.TypeVideo: {"mp4", "m4v", "webm", "mkv", "avi", "mov", "wmv", "flv", "mpg", "mpeg", "3gp", "ogv", "ts", "m2ts"},
	storage.TypeAudio: {
		"mp3", "wav", "ogg", "oga", "opus", "flac", "m4a", "aac", "wma", "aiff", "aif",
		"mid", "midi", "ape", "wv", "mka", "ac3", "eac3", "amr", "au", "snd", "voc", "ra",
	},
	storage.TypeImage: {"jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "avif", "ico", "tiff", "tif", "heic", "heif", "jxl"},
	storage.TypePDF:   {"pdf", "ai"},
	storage.TypeText: {
		// code
		"go", "ts", "tsx", "js", "jsx", "mjs", "cjs", "py", "rb", "rs", "java", "kt", "kts",
		"c", "h", "cpp", "cc", "cxx", "hpp", "hh", "cs", "php", "phps", "pl", "pm", "lua", "sql",
		"r", "swift", "dart", "scala", "groovy", "clj", "cljs", "ex", "exs", "erl", "hrl",
		"hs", "ml", "mli", "fs", "fsx", "vb", "asm", "s", "zig", "nim", "v", "jl", "cr",
		"dart", "ino", "pde", "sol", "sv", "vhd", "verilog",
		// web & markup
		"html", "htm", "css", "scss", "sass", "less", "styl", "vue", "svelte", "astro",
		"xml", "xsl", "xslt", "dtd", "md", "markdown", "mdx", "adoc", "rst", "tex", "org",
		"graphql", "gql", "proto", "thrift", "capnp",
		// config & data
		"json", "json5", "jsonc", "yaml", "yml", "toml", "ini", "cfg", "conf", "properties",
		"env", "editorconfig", "npmrc", "nvmrc", "babelrc", "prettierrc", "eslintrc",
		"gitconfig", "gitignore", "gitattributes", "dockerignore", "lock", "csv", "tsv",
		"hcl", "tf", "tfvars", "nix", "reg", "policy",
		// plain text: extension-only detection matters for empty files,
		// which have no content to sniff
		"txt", "text", "log", "nfo", "diz", "readme", "license", "changes",
		"authors", "install", "notice", "changelog", "contributing", "todo",
		// structured data formats that are JSON or XML underneath
		"har", "geojson", "topojson", "kml", "gml", "gfs", "rss", "atom", "plist", "resx",
		// shell & scripting
		"sh", "bash", "zsh", "fish", "ksh", "csh", "ps1", "psm1", "psd1", "bat", "cmd",
		"awk", "sed", "vim", "el",
		// build & ci
		"makefile", "mk", "cmake", "gradle", "sbt", "bazel", "bzl", "ninja", "dockerfile",
		"containerfile", "jenkinsfile", "gitlab-ci", "patch", "diff",
	},
}

// xmlMime is the generic MIME type for XML-structured documents whose
// specific types the browser would only download anyway.
const xmlMime = "application/xml"

const (
	jsMime   = "text/javascript"
	jsonMime = "application/json"
)

// mimeOverrides fills gaps in the operating system's mime tables. The .js
// family is overridden unconditionally: on Windows the mime package reads
// the registry, where scripts are frequently mapped to text/plain, which
// breaks strict MIME checks for module scripts (e.g. pdf.js workers).
var mimeOverrides = map[string]string{
	"js": jsMime, "mjs": jsMime, "cjs": jsMime,
	"json": jsonMime, "har": jsonMime, "topojson": jsonMime,
	"geojson": "application/geo+json",
	"kml":     "application/vnd.google-earth.kml+xml", "gml": "application/gml+xml",
	"gfs": xmlMime, "plist": xmlMime, "resx": xmlMime, "xsd": xmlMime,
	"rss": "application/rss+xml", "atom": "application/atom+xml",
	"mkv": "video/x-matroska", "avi": "video/x-msvideo", "mov": "video/quicktime",
	"mka":  "audio/x-matroska",
	"flac": "audio/flac", "opus": "audio/ogg", "m4a": "audio/mp4", "wma": "audio/x-ms-wma",
	"mid": "audio/midi", "midi": "audio/midi", "wv": "audio/x-wavpack", "ape": "audio/x-ape",
	"ac3": "audio/ac3", "eac3": "audio/eac3", "amr": "audio/amr",
	"au": "audio/basic", "snd": "audio/basic", "voc": "audio/x-voc",
	"ra":   "audio/x-pn-realaudio",
	"avif": "image/avif", "heic": "image/heic", "heif": "image/heif", "jxl": "image/jxl",
	"svg": "image/svg+xml", "ico": "image/x-icon", "tif": "image/tiff", "tiff": "image/tiff",
	"webp": "image/webp", "bmp": "image/bmp",
	"ai": "application/pdf",
	"md": "text/markdown", "yaml": "text/yaml", "yml": "text/yaml", "toml": "text/plain",
	"csv": "text/csv", "tsv": "text/tab-separated-values", "log": "text/plain",
	"go": "text/x-go", "rs": "text/x-rust", "ts": "text/x-typescript", "tsx": "text/x-typescript",
	"jsx": "text/x-javascript", "sh": "text/x-shellscript", "ps1": "text/x-powershell",
	"php": "text/x-php", "phps": "text/x-php",
	"dockerfile": "text/x-dockerfile", "makefile": "text/x-makefile",
}

func init() {
	for t, exts := range groups {
		for _, e := range exts {
			extKinds[e] = kind{typ: t}
		}
	}

	// Pin the script/wasm MIME types in the global table too: embedded SPA
	// assets are served via http.ServeContent, which consults it by file
	// extension. On Windows the registry maps scripts to text/plain and
	// browsers then refuse to execute them ("strict MIME type checking").
	for _, mt := range []struct{ ext, typ string }{
		{".js", jsMime},
		{".mjs", jsMime},
		{".cjs", jsMime},
		{".wasm", "application/wasm"},
	} {
		_ = mime.AddExtensionType(mt.ext, mt.typ)
	}
}

// sniffs maps content-type prefixes returned by http.DetectContentType to types.
var sniffs = []struct {
	prefix string
	typ    storage.FileType
}{
	{"image/", storage.TypeImage},
	{"video/", storage.TypeVideo},
	{"audio/", storage.TypeAudio},
	{"application/pdf", storage.TypePDF},
	{"text/", storage.TypeText},
	{"application/json", storage.TypeText},
	{"application/xml", storage.TypeText},
	{"application/javascript", storage.TypeText},
	{"application/x-yaml", storage.TypeText},
	{"application/toml", storage.TypeText},
}

// Detect classifies a file by name, falling back to a content sniff of the
// first bytes when the extension is unknown. head may be nil (extension-only
// detection, used for listings). Returned mime is never empty for files.
func Detect(name string, head []byte) (storage.FileType, string) {
	base := strings.ToLower(path.Base(name))

	if e := strings.TrimPrefix(path.Ext(base), "."); e != "" {
		if k, ok := extKinds[e]; ok {
			return k.typ, mimeTypeFor(e)
		}
		// Files without a recognized extension but with a known base name
		// (Dockerfile, Makefile, ...) are treated as text.
		if _, ok := extKinds[base]; ok {
			return storage.TypeText, mimeTypeFor(base)
		}
	} else if k, ok := extKinds[base]; ok {
		return k.typ, mimeTypeFor(base)
	}

	if len(head) > 0 {
		ct := http.DetectContentType(head)
		for _, s := range sniffs {
			if strings.HasPrefix(ct, s.prefix) {
				return s.typ, ct
			}
		}
	}
	return storage.TypeBlob, "application/octet-stream"
}

func mimeTypeFor(ext string) string {
	if m, ok := mimeOverrides[ext]; ok {
		return m
	}
	if m := mime.TypeByExtension("." + ext); m != "" {
		return m
	}
	return "application/octet-stream"
}
