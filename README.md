# Media Manager

A rebranded build of [Mediarium](https://github.com/rdborg/Mediarium): movies, TV, music and
books, from search to library, in one app. All credit for the software goes to the upstream
project. Licensed AGPL-3.0 (see [LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)).

`main` is upstream's code plus the files below, minus the features listed under
[What this fork removes](#what-this-fork-removes). The app itself is documented in [docs/](docs/README.md).

## What this fork adds

- `rebrand/apply.sh <checkout>` rebrands a checkout in place: logo, icons, share images, the
  product name in the UI, translations, docs and server messages, and links to this repo.
  Identifiers, env vars (`MEDIARIUM_*`), paths, headers and user agents stay as they are, so
  behaviour and stored data stay compatible. `rebrand/assets/` holds the rendered icons;
  `rebrand/make-assets.mjs` regenerates them.
- `.github/workflows/deploy.yml` runs on every push to `main` (or by hand): rebrands the
  checkout, pushes `ghcr.io/sheldonbakker/media-manager:latest` and `:<sha>`, joins the
  tailnet and deploys `:<sha>` to the VM over SSH.
- `deploy/docker-compose.yml` is the compose file the VM runs.

## What this fork removes

- The "Report a problem or share an idea" window (sidebar bug button, profile menu item, the
  nudge's "Send an idea") and its `POST /api/feedback` relay to mediarium.app.
- The ko-fi "Support Mediarium" sidebar link.
- The in-app user guide (`/help`, the `?` key and buttons, "Learn more" links, the
  docs-to-guide build step). The pages in `docs/` stay as documentation.
- Settings > System > About and credits (`/settings/about`, `/about`), including its credits,
  thank-you list, TMDB logo and legal notice.

## Deploy setup

On the VM, in `/opt/docker/<VM_DOCKER_FOLDER>` (default `/opt/docker/media-manager`):

- copy `deploy/docker-compose.yml` there;
- the SSH user must be in the `docker` group;
- if the GHCR package is private, run `docker login ghcr.io` once.

In GitHub (the `production` environment, or repo level):

| Name | Kind | Value |
| --- | --- | --- |
| `VM_HOST` | secret | the VM's Tailscale host name |
| `VM_USER` | secret | SSH user |
| `VM_SSH_KEY` | secret | private key for that user |
| `TS_OAUTH_CLIENT_ID`, `TS_OAUTH_SECRET` | secret | Tailscale OAuth client allowed to tag `tag:ci-cd` |
| `STACK_ENV` | variable | contents of the VM's `.env` (see the example in `deploy/docker-compose.yml`); required |
| `VM_DOCKER_FOLDER` | variable | optional, default `media-manager` |
| `IMAGE_NAME` | variable | optional, default `media-manager` |

## Updating from upstream

```bash
git fetch upstream --tags
git merge <upstream tag>     # for example v3.9.5
```

Files removed from this fork may come back as "deleted by us" conflicts; resolve those with
`git rm <file>`. Upstream changes to the removed features (above) are dropped the same way. Push to `main` and the workflow builds and deploys it.
