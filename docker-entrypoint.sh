#!/bin/sh
set -eu

data_dir="${MEOWTOPO_DATA_DIR:-/data}"

if [ "$(id -u)" = "0" ]; then
  mkdir -p "$data_dir"
  chown -R meowtopo:meowtopo "$data_dir"
  exec su-exec meowtopo:meowtopo /usr/local/bin/meowtopo "$@"
fi

exec /usr/local/bin/meowtopo "$@"
