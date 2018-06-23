#!/usr/bin/env bash

set -e

readonly date="$(date --utc +%Y.%m.%d~%H.%M)"
readonly tmpdir="$(mktemp -d)"

#go build 2>&1 | grep package | cut -d'"' -f2 | xargs go get -u
go build -o "${tmpdir}/OffsiteZFSBackup"
upx --ultra-brute "${tmpdir}/OffsiteZFSBackup"
chmod +rx ${tmpdir}
chmod +s "${tmpdir}/OffsiteZFSBackup"
fpm -n offsite-zfs-backup -v $date -s dir -t pacman -C "${tmpdir}" .=/opt/ozb
fpm -n offsite-zfs-backup -v $date -s dir -t deb -C "${tmpdir}" .=/opt/ozb
if [ -n "${tmpdir}" ]; then
  rm -f "${tmpdir}"
fi
