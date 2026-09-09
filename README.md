# filebrowser

A web file browser served by a single Go binary with an embedded React app.
Anyone can browse, upload, and download. Optional accounts add
file management: create, rename, move, delete, and private folders. Account
data lives in a SQLite database; files stay on the plain filesystem.

![filebrowser](.github/assets/screenshot.png)

## Quick start

Build from source (requires Go 1.26+ and Bun):

```bash
just build
FILEBROWSER_ROOT=./data ./bin/filebrowser
```

Or run the Docker image:

```bash
docker run -p 8080:8080 -v "$PWD/data:/data" -v "$PWD/db:/db" ghcr.io/skidoodle/filebrowser:latest
```

Then open http://localhost:8080. The first visit offers to create an admin
account.

## Configuration

All settings come from environment variables.

| Variable | Default | Meaning |
| --- | --- | --- |
| `FILEBROWSER_ROOT` | `./data` | directory served to visitors |
| `FILEBROWSER_DATABASE` | `./filebrowser.db` | path to SQLite database |
| `FILEBROWSER_ADDRESS` | `0.0.0.0:8080` | listen address |
| `FILEBROWSER_BASEURL` | empty | subpath the app is mounted under |
| `FILEBROWSER_ACCESS_POLICY` | `public` | `public` lets anonymous visitors upload, `readonly` limits them to browsing, `private` requires sign-in for everything |
| `FILEBROWSER_MAXUPLOAD` | `10GiB` | maximum size of a single upload |
| `FILEBROWSER_CACHEDIR` | system temp | directory for thumbnails and other disposable data |
| `FILEBROWSER_MAXTEXTSIZE` | `10MiB` | cutoff for text type detection |
| `FILEBROWSER_GUARD` | `true` | toggle the abuse-protection chain |
| `FILEBROWSER_TRUSTEDPROXIES` | empty | comma-separated proxy CIDRs for `X-Forwarded-For` |
| `FILEBROWSER_REQUESTRATE` | `60` | per-IP API requests per second |
| `FILEBROWSER_DOWNLOADRATE` | `200MiB` | per-IP download bytes per second |
| `FILEBROWSER_POWDIFFICULTY` | `4` | proof-of-work difficulty, 0 disables it |
| `FILEBROWSER_SECRET` | empty | capability signing key; defaults to a key persisted in the cache dir |
| `FILEBROWSER_INSECURE` | `false` | disable authentication, give every visitor full access |
| `FILEBROWSER_ADMIN_PASSWORD` | empty | seed or reset the admin password |
| `FILEBROWSER_DEBUG` | `false` | verbose logging |

## Development

```bash
just dev      # API on :8080 and Vite dev server on :5173
just test     # Go tests with the race detector
just lint     # golangci-lint
just fmt      # gofmt and go vet
just build    # production binary with the embedded SPA
```

Run `just --list` for the rest. The backend is Go 1.26+ on the standard
library. The frontend is React 19, TypeScript, Vite, and Tailwind CSS, built
with Bun.

## License

BSD 3-Clause. See [LICENSE](LICENSE).
