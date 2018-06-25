#!/usr/bin/env bash

set -e

readonly date="$(date --utc +%Y.%m.%d~%H.%M.%S)"
readonly tmpdir="$(mktemp -d)"

#go build 2>&1 | grep package | cut -d'"' -f2 | xargs go get -u
GOARCH=arm GOARM=5 go build -o "${tmpdir}/OffsiteZFSBackup"
upx --ultra-brute "${tmpdir}/OffsiteZFSBackup"
chmod +rx ${tmpdir}
chmod +s "${tmpdir}/OffsiteZFSBackup"
fpm -n offsite-zfs-backup4arm -v $date -s dir -t pacman -C "${tmpdir}" .=/opt/ozb
xz -vd *.pkg.tar.xz
gzip -v9 *.pkg.tar
scp *.pkg.tar.gz mirror.t4cc0.re:/vault/packages
rm *.pkg.tar.gz || true
if [ -n "${tmpdir}" ]; then
  rm -r "${tmpdir}"
fi

cd -
