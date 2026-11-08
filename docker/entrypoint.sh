#!/bin/sh
set -e

say() { echo "Mediarium: $*" >&2; }

# ---- UMASK ---------------------------------------------------------------------
# UMASK works as in the linuxserver.io images: an optional octal value (for
# example 022 or 002) that is set before the app starts, so the files and
# folders it creates get the permissions you want. Like there, it takes
# permissions away from the usual 666 for files and 777 for folders: 022 gives
# files 644 and folders 755, 002 gives 664 and 775. Unset leaves the container's
# own umask (022) alone. A value that is not one to four octal digits is ignored
# with a note. The permissions can also be set in Settings > Library > Folders
# and file names, which applies them to what Mediarium puts in the library and
# does not depend on the umask.
apply_umask() {
  [ -n "${UMASK:-}" ] || return 0
  case "$UMASK" in
    *[!0-7]*) say "ignoring UMASK=$UMASK: it has to be octal digits only, for example 022."; return 0 ;;
  esac
  if [ "${#UMASK}" -gt 4 ]; then
    say "ignoring UMASK=$UMASK: it has to be at most four octal digits, for example 022."
    return 0
  fi
  umask "$UMASK" || say "could not set UMASK=$UMASK."
}

# Started as a non-root user already (for example "user: 1000:1000" in
# compose, Kubernetes, or an app platform that sets the user itself): PUID and
# PGID do not apply, and nothing can be chowned. Create missing folders if
# possible, say which ones cannot be written, and start the app as this user.
if [ "$(id -u)" != "0" ]; then
  for dir in "${CONFIG_DIR:-/config}" "${DOWNLOADS_DIR:-/downloads}" "${MOVIES_DIR:-/movies}" "${TV_DIR:-/tv}" ${MUSIC_DIR:+"$MUSIC_DIR"} ${EBOOKS_DIR:+"$EBOOKS_DIR"} ${AUDIOBOOKS_DIR:+"$AUDIOBOOKS_DIR"}; do
    [ -d "$dir" ] || mkdir -p "$dir" 2>/dev/null || true
    if [ ! -w "$dir" ]; then
      echo "WARNING: $dir is not writable by uid $(id -u)/gid $(id -g). Fix the folder permissions on the host." >&2
    fi
  done
  apply_umask
  exec "$@"
fi

# PUID and PGID are numbers. Anything else could not start the app anyway; say
# why instead of failing further down.
for id_name in PUID PGID; do
  eval "id_value=\${$id_name:-}"
  case "$id_value" in
    ''|*[!0-9]*)
      echo "ERROR: $id_name must be a number (the output of \"id -u\" or \"id -g\" for the user who owns your media folders), but it is set to \"$id_value\". Fix it in your compose file." >&2
      exit 1
      ;;
  esac
done

# run_as runs a command as PUID:PGID. The small Alpine image has su-exec; the
# "full" image is Debian-based and uses setpriv (numeric ids need no user entry).
if command -v su-exec >/dev/null 2>&1; then
  run_as() { su-exec "${PUID}:${PGID}" "$@"; }
  # Match the app user to PUID/PGID so files written to the mounted folders
  # have the ownership your NAS/host expects.
  addgroup -g "${PGID}" -S appgroup 2>/dev/null || true
  adduser -u "${PUID}" -S -G appgroup appuser 2>/dev/null || true
else
  run_as() { setpriv --reuid="${PUID}" --regid="${PGID}" --clear-groups "$@"; }
fi

# /config is Mediarium's own private state, so it is safe to (re)own it.
mkdir -p /config
# -h: a link inside /config is changed itself, never what it points at.
chown -hR "${PUID}:${PGID}" /config 2>/dev/null || true
# The chown above can fail quietly (a network share with root squash, a host
# folder that does not allow it). Say so now, instead of leaving the first sign
# to be a database that "can't be opened".
if ! run_as test -w /config; then
  echo "WARNING: /config is not writable by uid ${PUID}/gid ${PGID}, so Mediarium cannot save its settings or database there. Fix the folder permissions on the host (on a network share, check that it allows this user to write), or change PUID/PGID." >&2
fi

# is_mapped DIR: true when DIR, or a folder above it, is mounted from the host
# (for example /data/movies inside a /data mount).
is_mapped() {
  d="$1"
  while [ -n "$d" ] && [ "$d" != "/" ]; do
    if awk -v p="$d" '$5 == p { found = 1 } END { exit !found }' /proc/self/mountinfo; then
      return 0
    fi
    d="$(dirname "$d")"
  done
  return 1
}

# is_fresh DIR: an empty folder owned by root, which is what Docker creates when
# the host folder you mapped did not exist yet. Nothing of yours is in it.
is_fresh() {
  [ -d "$1" ] && [ -z "$(ls -A "$1" 2>/dev/null)" ] && [ "$(stat -c %u "$1")" = "0" ]
}

# Your media folders are NEVER chowned or modified recursively here: they hold
# your existing files. A missing folder is created as the app user, a brand-new
# empty folder Docker made for you is handed to the app user (that one folder
# only), and anything else that is not writable is reported and left alone.
# The folders checked are the ones the app uses: DOWNLOADS_DIR, MOVIES_DIR and
# TV_DIR (by default /downloads, /movies and /tv, or /data/downloads,
# /data/movies and /data/tv with one /data mount). MUSIC_DIR, EBOOKS_DIR and
# AUDIOBOOKS_DIR are checked the same way, but only when you set them.
for dir in "${DOWNLOADS_DIR:-/downloads}" "${MOVIES_DIR:-/movies}" "${TV_DIR:-/tv}" ${MUSIC_DIR:+"$MUSIC_DIR"} ${EBOOKS_DIR:+"$EBOOKS_DIR"} ${AUDIOBOOKS_DIR:+"$AUDIOBOOKS_DIR"}; do
  if [ ! -d "$dir" ]; then
    parent="$(dirname "$dir")"
    if [ "$parent" = "/" ]; then
      # A top-level folder inside the container itself (nothing mapped there).
      mkdir -p "$dir" && chown "${PUID}:${PGID}" "$dir"
    else
      if [ "${PUID}" != "0" ] && is_fresh "$parent"; then
        chown "${PUID}:${PGID}" "$parent"
      fi
      run_as mkdir -p "$dir" 2>/dev/null || true
    fi
  fi

  if [ ! -d "$dir" ]; then
    echo "WARNING: could not create $dir as uid ${PUID}/gid ${PGID}. Make sure that user can write to $(dirname "$dir") on the host, or change PUID/PGID." >&2
    continue
  fi

  if ! run_as test -w "$dir"; then
    if is_fresh "$dir"; then
      chown "${PUID}:${PGID}" "$dir"
    else
      echo "WARNING: $dir is not writable by uid ${PUID}/gid ${PGID}. Fix the folder permissions on the host or change PUID/PGID; Mediarium will not touch it." >&2
      continue
    fi
  fi

  if ! is_mapped "$dir"; then
    echo "NOTE: $dir is not mapped to a folder on your host, so anything saved there is lost when the container is recreated. Map it in your compose file." >&2
  fi
done

apply_umask
if command -v su-exec >/dev/null 2>&1; then
  exec su-exec "${PUID}:${PGID}" "$@"
fi
exec setpriv --reuid="${PUID}" --regid="${PGID}" --clear-groups "$@"

