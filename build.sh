#!/usr/bin/env bash

readonly date="$(date --utc +%Y.%m.%d~%H.%M.%S)"

ToolExists() {
  if ! hash $1 &>/dev/null; then
    echo $1 is not installed
    exit 1
  fi
}

pack () {
  upx --best --ultra-brute --all-methods --overlay=strip $*
}

compile () {
  go build --ldflags="-s -w" -a -v $*
}

ToolExists go
ToolExists date
ToolExists chmod
ToolExists chown
ToolExists upx
ToolExists fpm
ToolExists mktemp
ToolExists xz
ToolExists gzip
ToolExists scp
ToolExists rm

set -e

rm *.deb *.pkg.tar.?z || true

#go build 2>&1 | grep package | cut -d'"' -f2 | xargs go get -u
### x86_64
echo "Building x86_64..."
tmpdir="$(mktemp -d)"

echo "Compiling..."
compile -o "${tmpdir}/OffsiteZFSBackup"
echo "Compressing..."
pack "${tmpdir}/OffsiteZFSBackup"
chmod +rx ${tmpdir}
chmod +s "${tmpdir}/OffsiteZFSBackup"
echo "Building packages..."
fpm -n offsite-zfs-backup -v $date -s dir -t pacman -a x86_64 -C "${tmpdir}" .=/opt/ozb
fpm -n offsite-zfs-backup -v $date -s dir -t deb -a x86_64 -C "${tmpdir}" .=/opt/ozb

### ARM
echo "Building ARM..."
tmpdir="$(mktemp -d)"

echo "Compiling..."
GOARCH=arm GOARM=5 compile -o "${tmpdir}/OffsiteZFSBackup"
echo "Compressing..."
pack "${tmpdir}/OffsiteZFSBackup"
chmod +rx ${tmpdir}
chmod +s "${tmpdir}/OffsiteZFSBackup"
echo "Building packages..."
fpm -n offsite-zfs-backup -v $date -s dir -t deb -a armv6l -C "${tmpdir}" .=/opt/ozb
fpm -n offsite-zfs-backup -v $date -s dir -t pacman -a armv6l -C "${tmpdir}" .=/opt/ozb
xz -vd *.pkg.tar.xz
gzip -v9 *.pkg.tar

### Upload
echo "Uploading..."
scp -oProxyJump=jumper *.deb *.pkg.tar.?z blaze.t4cc0.re:/vault/packages
echo "Cleaning..."
rm *.deb *.pkg.tar.?z || true
if [ -n "${tmpdir}" ]; then
  rm -r "${tmpdir}"
fi
