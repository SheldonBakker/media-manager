#!/usr/bin/env bash
# Rebrands an upstream Mediarium checkout as "Media Manager".
# Usage: rebrand/apply.sh <path-to-upstream-checkout>
# Idempotent. Fails loudly when an upstream change means a pattern no longer matches,
# so a silent half-rebrand never gets built.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SRC="${1:?usage: apply.sh <upstream checkout>}"
cd "$SRC"

NAME='Media Manager'
REPO_URL='github.com/SheldonBakker/media-manager'
REPO_SLUG='SheldonBakker/media-manager'
IMAGE='ghcr.io/sheldonbakker/media-manager'
ARCH='M5 28V15a11 11 0 0 1 22 0v13h-6l-5-11-5 11z'
SHINE='M7.4 15.5a8.6 8.6 0 0 1 7.2-8.1'
MARK='M7 5h18a4 4 0 0 1 4 4v14a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4V9a4 4 0 0 1 4-4zM13 10.5v11l9-5.5z'
NEW_SHINE='M5.6 10a3.4 3.4 0 0 1 2.9-2.9'

die() { echo "rebrand: $*" >&2; exit 1; }
# rep FILE PERL-EXPR: apply a perl substitution in place, failing if the file is missing.
rep() { [ -f "$1" ] || die "missing $1"; perl -0pi -e "$2" "$1"; }
export ARCH SHINE MARK NEW_SHINE NAME

# 1. Logo mark (shape only; colours stay the app's own accent).
for f in web/index.html web/src/assets/mediarium-mark.svg; do
  [ -f "$f" ] || continue
  rep "$f" 's/d="\Q$ENV{ARCH}\E"/fill-rule="evenodd" d="$ENV{MARK}"/g; s/\Q$ENV{SHINE}\E/$ENV{NEW_SHINE}/g'
done
rep web/src/components/BrandMark.tsx 's/d="\Q$ENV{ARCH}\E"/fillRule="evenodd" d="$ENV{MARK}"/g'
rep web/src/components/LogoGlyph.tsx 's/\Q$ENV{ARCH}\E/$ENV{MARK}/g; s/\Q$ENV{SHINE}\E/$ENV{NEW_SHINE}/g; s/(?<!fillRule="evenodd" )d=\{arch\}/fillRule="evenodd" d={arch}/g'

# 2. Icons, favicons and share images.
for f in favicon.svg favicon.ico favicon-16.png favicon-32.png favicon-48.png apple-touch-icon-180.png icon-192.png icon-512.png icon-maskable-512.png; do
  cp "$HERE/assets/$f" "web/public/$f"
done
rm -f web/public/share/mediarium-*.jpg
for s in wide square story; do cp "$HERE/assets/share-$s.png" "web/public/share/share-$s.png"; done
rep web/src/share.ts 's#/share/mediarium-(wide|square|story)\.jpg#/share/share-$1.png#g'
grep -q 'mediarium-wide' web/src/share.ts && die "share.ts still points at old images"
if grep -rIl --exclude-dir=node_modules --exclude-dir=dist -F "$ARCH" web >/dev/null; then
  die "old logo shape still present in: $(grep -rIl --exclude-dir=node_modules --exclude-dir=dist -F "$ARCH" web | tr '\n' ' ')"
fi

# 3. Links to the upstream repo and image now point at the fork.
TEXT_DIRS=(web/src web/index.html web/public docs internal cmd docker)
mapfile -t FILES < <(grep -rIl --exclude-dir=node_modules --exclude-dir=dist --exclude-dir=testdata \
  -e 'Mediarium' -e 'rdborg' -e 'mediarium\.app' "${TEXT_DIRS[@]}" 2>/dev/null || true)
[ "${#FILES[@]}" -gt 0 ] || die "no files to rebrand (wrong path?)"
export REPO_URL REPO_SLUG IMAGE
perl -pi -e '
  s#github\.com/rdborg/Mediarium#$ENV{REPO_URL}#g;
  s#github\\\.com\\/rdborg\\/Mediarium#github\\.com\\/SheldonBakker\\/media-manager#g;
  s#ghcr\.io/rdborg/mediarium#$ENV{IMAGE}#g;
' "${FILES[@]}"
rep internal/updatecheck/updatecheck.go 's/var DefaultRepo = "rdborg\/Mediarium"/var DefaultRepo = "$ENV{REPO_SLUG}"/'
grep -q "DefaultRepo = \"$REPO_SLUG\"" internal/updatecheck/updatecheck.go || die "update check repo not switched"

# Upstream's website and Reddit links leave the profile menu (they are about upstream, not this build).
rep web/src/components/ProfileMenu.tsx 's#\s*<a href=\{(WEBSITE_URL|REDDIT_URL)\}[^\n]*\n[^\n]*\n\s*</a>##g; s/(?:,\s*(?:WEBSITE_URL|REDDIT_URL)\b)|(?:\b(?:WEBSITE_URL|REDDIT_URL),\s*)//g'
grep -qE 'WEBSITE_URL|REDDIT_URL' web/src/components/ProfileMenu.tsx && die "ProfileMenu still links upstream website/reddit"
perl -pi -e 's#/Mediarium/blob/#/media-manager/blob/#g' "${FILES[@]}"

# 4. The name. Only the standalone word: identifiers (MediariumSettings), env vars
#    (MEDIARIUM_*), paths, header names and user agents (Mediarium/, /Mediarium,
#    X-Mediarium-) are left alone so nothing functional changes.
perl -pi -e 's/(?<![\w\/\-=\\.@])Mediarium(?=s\b|(?![\w\/\-\\]))/$ENV{NAME}/g' "${FILES[@]}"
# Heading anchors follow the renamed headings.
perl -pi -e 's/#mediarium-(?=[a-z0-9])/#media-manager-/g' "${FILES[@]}"
perl -pi -e "s/'mediarium-is-slow-to-answer/'media-manager-is-slow-to-answer/g" web/src/guide/markdown.test.ts
# The share text pointed people at the upstream website.
perl -pi -e 's/More at mediarium\.app/More at $ENV{REPO_URL}/g' "${FILES[@]}"

# 5. Image metadata.
perl -pi -e 's#org\.opencontainers\.image\.source="https://github\.com/rdborg/mediarium"#org.opencontainers.image.source="https://$ENV{REPO_URL}"#g; s/description="Mediarium /description="$ENV{NAME} /g; s/echo "Building Mediarium /echo "Building $ENV{NAME} /' Dockerfile

left=$(grep -rIoP --exclude-dir=node_modules --exclude-dir=dist '(?<![\w/\-=\\.@])Mediarium(?![\w/\-\\])' web/src web/index.html web/public | wc -l || true)
[ "$left" -eq 0 ] || die "$left visible name occurrences left in web/"
echo "rebrand: done"
