# Media Manager rebrand

This fork builds a rebranded image of [Mediarium](https://github.com/rdborg/Mediarium)
(AGPL-3.0) called "Media Manager", with its own name and logo as the upstream README asks
of shared, changed versions. All credit for the software goes to the upstream project.

- `apply.sh <checkout>` rebrands an upstream checkout in place: logo shape, icons, share
  images, the product name in the UI, translations, docs and server messages, and links to
  this repo. Identifiers, env vars (`MEDIARIUM_*`), paths, headers and user agents are left
  alone so behaviour and stored data stay compatible.
- `assets/` holds the rendered icons; `make-assets.mjs` regenerates them.
- `.github/workflows/rebrand-build.yml` runs daily: takes the latest upstream release, runs
  `apply.sh`, runs the web tests, pushes the exact rebranded source to the `branded` branch,
  and publishes `ghcr.io/sheldonbakker/media-manager:<version>` and `:latest`.
- `.github/workflows/deploy.yml` runs on every push to `main`: rebrands the checkout, pushes
  `:latest` and `:<sha>`, then deploys `:<sha>` to the VM over Tailscale using
  `deploy/docker-compose.yml` (in `/opt/docker/<VM_DOCKER_FOLDER>` on the VM).

The `main` branch is upstream's code plus this folder; it is not merged with upstream.
The source of each image is the `branded` branch commit named in its release.
