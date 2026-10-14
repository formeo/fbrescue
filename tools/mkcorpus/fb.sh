#!/usr/bin/env bash
# fb.sh <version> <tool> [args...] runs a Firebird tool (isql, gstat, gfix,
# nbackup) of version 25, 30, 40 or 50 in embedded mode, from the engines
# that mkcorpus.sh downloads into $FB_CACHE (default ~/.cache/fbrescue/firebird).
set -euo pipefail
CACHE=${FB_CACHE:-$HOME/.cache/fbrescue/firebird}
v=$1 tool=$2
shift 2
root=$CACHE/$v/opt/firebird
[[ -x $root/bin/$tool ]] || { echo "fb.sh: Firebird $v is not installed in $CACHE; run tools/mkcorpus/mkcorpus.sh" >&2; exit 127; }
mkdir -p "$CACHE/lock$v" "$CACHE/tmp$v"
FIREBIRD=$root FIREBIRD_LOCK=$CACHE/lock$v FIREBIRD_TMP=$CACHE/tmp$v \
LD_LIBRARY_PATH=$root/lib:$CACHE/compat ISC_USER=SYSDBA ISC_PASSWORD=masterkey \
  exec "$root/bin/$tool" "$@"
