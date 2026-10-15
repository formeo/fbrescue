#!/usr/bin/env bash
# Builds the test corpus: databases created by real Firebird engines
# (2.5, 3.0, 4.0, 5.0) in embedded mode, with `gstat -h` and `gstat -r`
# output for each as references. Tests also run the engines through fb.sh.
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
fb() { FB_CACHE=$CACHE "$(dirname "$0")/fb.sh" "$@"; }

create_db() { # create_db <version> <path> <dialect> [page size] [extra clause]
  echo "create database '$2' page_size ${4:-8192} ${5:-}; quit;" | fb "$1" isql -q -sql_dialect "$3"
}

# patch <file> <offset> <hex bytes> writes bytes into a file in place.
patch() { printf "$(sed 's/\([0-9a-f][0-9a-f]\)/\\x\1/g' <<<"$3")" | dd of="$1" bs=1 seek=$(($2)) conv=notrunc status=none; }

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
  cp "$d/base.fdb" "$d/shutmulti.fdb"; fb "$v" gfix -shut multi -force 0 "$d/shutmulti.fdb"
  cp "$d/base.fdb" "$d/shutfull.fdb"; fb "$v" gfix -shut full -force 0 "$d/shutfull.fdb"
  cp "$d/base.fdb" "$d/noreserve.fdb"; fb "$v" gfix -use full "$d/noreserve.fdb"
  create_db "$v" "$d/dialect1.fdb" 1
  if [[ $v == 40 || $v == 50 ]]; then
    cp "$d/base.fdb" "$d/replica_ro.fdb"; fb "$v" gfix -replica read_only "$d/replica_ro.fdb"
    cp "$d/base.fdb" "$d/replica_rw.fdb"; fb "$v" gfix -replica read_write "$d/replica_rw.fdb"
  fi

  # High 16 bits of the transaction numbers (next, OIT, OAT, OST = 1, 2, 3, 4)
  # and of the attachment id (5); ODS 12+ only. gstat reads them back.
  case $v in
    30) cp "$d/base.fdb" "$d/hiwords.fdb"; patch "$d/hiwords.fdb" 0x78 050000000100020003000400 ;;
    40|50) cp "$d/base.fdb" "$d/hiwords.fdb"; patch "$d/hiwords.fdb" 0x74 050000000100020003000400 ;;
  esac

  # Over 16 304 transactions on 4K pages: three TIP pages.
  create_db "$v" "$d/manytx.fdb" 3 4096
  fb "$v" isql -q "$d/manytx.fdb" -i <(echo "create table c (i integer); commit;"
    for i in $(seq 17000); do echo "insert into c values ($i); commit;"; done) > /dev/null

  # A secondary file holding logical pages from 300 on.
  create_db "$v" "$d/multifile.fdb" 3 4096 "file '$d/multifile.f2' starting at page 300"
  fb "$v" isql -q "$d/multifile.fdb" -i "$SQL/multifile.sql"

  # Larger than one PIP range on 4K pages (32 608 / 32 544 pages), with
  # dozens of pointer pages. ~140 MB, so one engine per page layout family.
  if [[ $v == 25 || $v == 50 ]]; then
    create_db "$v" "$d/big.fdb" 3 4096
    fb "$v" isql -q "$d/big.fdb" -i "$SQL/big.sql"
  fi

  # Every type export handles, in a UTF8 database.
  create_db "$v" "$d/types.fdb" 3 8192 "default character set utf8"
  fb "$v" isql -q -ch UTF8 "$d/types.fdb" -i "$SQL/types.sql"
  [[ $v == 25 ]] || fb "$v" isql -q -ch UTF8 "$d/types.fdb" -i "$SQL/boolean.sql"

  # Record layouts: format changes, long runs, fragments, rolled-back
  # changes, blobs on many pages, a descriptor larger than a page, and
  # WIN1251 bytes ("Привет") in a CHARACTER SET NONE column.
  # Nothing may read these tables through the engine afterwards: that would
  # garbage-collect the rolled-back versions.
  create_db "$v" "$d/layout.fdb" 3 4096
  fb "$v" gfix -housekeeping 0 "$d/layout.fdb"
  fb "$v" isql -q "$d/layout.fdb" -i "$SQL/layout.sql"
  echo "create table wide ($(seq -f 'c%g smallint' -s, 1 700)); commit;
    insert into wide (c1, c350, c700) values (1, 350, 700); commit; quit;" | fb "$v" isql -q "$d/layout.fdb"
  echo "create table cs (id integer not null, n varchar(20) character set none); commit;
    insert into cs values (1, x'CFF0E8E2E5F2'); commit; quit;" | fb "$v" isql -q "$d/layout.fdb"

  # Record version chains: committed and rolled-back updates and deletes.
  # Housekeeping off, and nothing may read the table through the engine.
  create_db "$v" "$d/versions.fdb" 3 4096 "default character set utf8"
  fb "$v" gfix -housekeeping 0 "$d/versions.fdb"
  fb "$v" isql -q -ch UTF8 "$d/versions.fdb" -i "$SQL/versions.sql"

  # Blobs of every level (a level-2 one of 8 MB), text and binary, and on
  # Firebird 5 stream blobs.
  create_db "$v" "$d/blobs.fdb" 3 4096 "default character set utf8"
  fb "$v" isql -q -ch UTF8 "$d/blobs.fdb" -i "$SQL/blobs.sql"
  [[ $v != 50 ]] || fb "$v" isql -q -ch UTF8 "$d/blobs.fdb" -i "$SQL/blobs5.sql"

  # Types added in Firebird 4.
  if [[ $v == 40 || $v == 50 ]]; then
    create_db "$v" "$d/fb4types.fdb" 3 8192 "default character set utf8"
    fb "$v" isql -q -ch UTF8 "$d/fb4types.fdb" -i "$SQL/fb4types.sql"
  fi

  # nbackup lock with a committed row that lives only in nblock.fdb.delta.
  cp "$d/base.fdb" "$d/nblock.fdb"
  fb "$v" nbackup -L "$d/nblock.fdb"
  echo "insert into t (id, name) values (-1, 'written under nbackup lock'); commit; quit;" |
    fb "$v" isql -q "$d/nblock.fdb"

  for db in "$d"/*.fdb; do
    # gstat -r attaches to the database, which bumps the transaction and
    # attachment counters, so the header is read last. -r is not available
    # for databases in full shutdown.
    fb "$v" gstat -d -r -s "$db" > "${db%.fdb}.gstat-r.txt" 2>/dev/null || rm "${db%.fdb}.gstat-r.txt"
    fb "$v" gstat -h "$db" > "${db%.fdb}.gstat.txt"
  done
  echo "fb$v: $(ls "$d"/*.fdb | wc -l) databases" >&2
done
