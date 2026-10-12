#!/usr/bin/env bash
# Builds the test corpus: databases created by real Firebird engines
# (2.5, 3.0, 4.0, 5.0) in embedded mode, with `gstat -h` output for each
# as the reference.
#
# Usage: tools/mkcorpus/mkcorpus.sh [output-dir]   (default: corpus)
# Linux x86-64 only. Needs curl, tar and the libraries libtommath.so.1,
# libncurses.so.6, libtinfo.so.6 (Ubuntu: apt-get install libtommath1 libncurses6).
# Downloads are cached in $FB_CACHE (default ~/.cache/fbrescue/firebird).
set -euo pipefail

OUT=$(mkdir -p "${1:-corpus}" && cd "${1:-corpus}" && pwd)
CACHE=${FB_CACHE:-$HOME/.cache/fbrescue/firebird}
SQL=$(cd "$(dirname "$0")/sql" && pwd)
REL=https://github.com/FirebirdSQL/firebird/releases/download

declare -A URL=(
  [25]=$REL/R2_5_9/FirebirdCS-2.5.9.27139-0.amd64.tar.gz
  [30]=$REL/v3.0.12/Firebird-3.0.12.33787-0.amd64.tar.gz
  [40]=$REL/v4.0.5/Firebird-4.0.5.3140-0.amd64.tar.gz
  [50]=$REL/v5.0.1/Firebird-5.0.1.1469-0-linux-x64.tar.gz
)

# The release builds link older sonames than current distributions ship.
compat_libs() {
  mkdir -p "$CACHE/compat"
  local want have
  for pair in libtommath.so.0:libtommath.so.1 libncurses.so.5:libncurses.so.6 libtinfo.so.5:libtinfo.so.6; do
    want=${pair%%:*}; have=$(ldconfig -p | awk -v l="${pair##*:}" '$1 == l { print $NF; exit }')
    [[ -n $have ]] || { echo "missing system library ${pair##*:}" >&2; exit 1; }
    ln -sf "$have" "$CACHE/compat/$want"
  done
}

install_fb() {
  local v=$1 dir=$CACHE/$1
  [[ -x $dir/opt/firebird/bin/isql ]] && return
  local tmp; tmp=$(mktemp -d)
  echo "downloading ${URL[$v]##*/}" >&2
  curl -fsSL "${URL[$v]}" | tar xz -C "$tmp"
  mkdir -p "$dir"
  tar xzf "$tmp"/*/buildroot.tar.gz -C "$dir"
  rm -rf "$tmp"
}

# fb <version> <tool> [args...] runs a Firebird tool in embedded mode.
fb() {
  local v=$1 tool=$2; shift 2
  local root=$CACHE/$v/opt/firebird
  mkdir -p "$CACHE/lock$v" "$CACHE/tmp$v"
  FIREBIRD=$root FIREBIRD_LOCK=$CACHE/lock$v FIREBIRD_TMP=$CACHE/tmp$v \
  LD_LIBRARY_PATH=$root/lib:$CACHE/compat ISC_USER=SYSDBA ISC_PASSWORD=masterkey \
    "$root/bin/$tool" "$@"
}

create_db() { # create_db <version> <path> <dialect>
  echo "create database '$2' page_size 8192; quit;" | fb "$1" isql -q -sql_dialect "$3"
}

compat_libs
for v in 25 30 40 50; do
  install_fb "$v"
  d=$OUT/fb$v
  rm -rf "$d"; mkdir -p "$d"

  create_db "$v" "$d/base.fdb" 3
  fb "$v" isql -q "$d/base.fdb" -i "$SQL/base.sql"

  cp "$d/base.fdb" "$d/deleted.fdb"
  fb "$v" gfix -housekeeping 0 "$d/deleted.fdb"
  fb "$v" isql -q "$d/deleted.fdb" -i "$SQL/deleted.sql"

  cp "$d/base.fdb" "$d/async.fdb";    fb "$v" gfix -write async "$d/async.fdb"
  cp "$d/base.fdb" "$d/readonly.fdb"; fb "$v" gfix -mode read_only "$d/readonly.fdb"
  cp "$d/base.fdb" "$d/shutdown.fdb"; fb "$v" gfix -shut single -force 0 "$d/shutdown.fdb"
  create_db "$v" "$d/dialect1.fdb" 1

  # nbackup lock with a committed row that lives only in nblock.fdb.delta.
  cp "$d/base.fdb" "$d/nblock.fdb"
  fb "$v" nbackup -L "$d/nblock.fdb"
  echo "insert into t (id, name) values (-1, 'written under nbackup lock'); commit; quit;" |
    fb "$v" isql -q "$d/nblock.fdb"

  for db in "$d"/*.fdb; do
    fb "$v" gstat -h "$db" > "${db%.fdb}.gstat.txt"
  done
  echo "fb$v: $(ls "$d"/*.fdb | wc -l) databases" >&2
done
