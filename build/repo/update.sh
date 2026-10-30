#!/usr/bin/env bash
# Adds new .deb and .rpm files to the TuxShell package repository and signs it.
#
#   build/repo/update.sh SITE NEW
#
# SITE is a copy of the repository (what pkg.tuxshell.io serves; may be empty
# the first time), NEW a folder with the release's .deb and .rpm. Afterwards
# SITE holds the complete repository, ready to upload:
#
#   tuxshell.asc                      public signing key
#   apt/pool/main/t/tuxshell/*.deb
#   apt/dists/stable/{Release,InRelease,Release.gpg,main/binary-amd64/Packages*}
#   rpm/*.rpm                         each one signed
#   rpm/repodata/                     repomd.xml signed (repomd.xml.asc)
#   rpm/tuxshell.repo
#
# Only the newest KEEP versions stay. Needs gpg with the signing key, plus
# apt-ftparchive (apt-utils), createrepo_c and rpmsign (rpm).
# Environment: GPG_KEY_ID (required), GPG_PASSPHRASE_FILE (when the key has one).
set -euo pipefail

site=${1:?usage: update.sh SITE NEW}
new=${2:?usage: update.sh SITE NEW}
here=$(cd "$(dirname "$0")" && pwd)
keep=${KEEP:-10}
key=${GPG_KEY_ID:?set GPG_KEY_ID}

pass_args=()
if [[ -n ${GPG_PASSPHRASE_FILE:-} ]]; then
	pass_args=(--pinentry-mode loopback --passphrase-file "$GPG_PASSPHRASE_FILE")
fi
gpg_args=(--batch --yes --local-user "$key" "${pass_args[@]}")

pool=$site/apt/pool/main/t/tuxshell
dist=$site/apt/dists/stable
rpms=$site/rpm
mkdir -p "$pool" "$dist/main/binary-amd64" "$rpms"

shopt -s nullglob
debs=("$new"/*.deb)
newrpms=("$new"/*.rpm)
if ((${#debs[@]} + ${#newrpms[@]} == 0)); then
	echo "no .deb or .rpm in $new" >&2
	exit 1
fi
# A package already in the repository stays as it is: signing an .rpm again
# changes its bytes under the same name, which Cloudflare's cache would keep
# serving in its old form.
for f in "${debs[@]}"; do
	[[ -e $pool/$(basename "$f") ]] || cp -- "$f" "$pool"/
done
rpm_args=(--define "__gpg $(command -v gpg)" --define "_gpg_name $key")
if ((${#pass_args[@]})); then
	rpm_args+=(--define "_gpg_sign_cmd_extra_args ${pass_args[*]}")
fi
for f in "${newrpms[@]}"; do
	[[ -e $rpms/$(basename "$f") ]] && continue
	cp -- "$f" "$rpms"/
	rpmsign --addsign "${rpm_args[@]}" "$rpms/$(basename "$f")"
done

# Keep the newest $keep versions of each (names sort by version with -V).
prune() {
	local files
	mapfile -t files < <(printf '%s\n' "$@" | sort -V)
	((${#files[@]} > keep)) || return 0
	rm -f -- "${files[@]:0:${#files[@]}-keep}"
}
prune "$pool"/*.deb
prune "$rpms"/*.rpm

# apt: package index, then the signed Release file (written outside the
# folder it describes).
tmp=$(mktemp)
rm -f -- "$dist/Release" "$dist/InRelease" "$dist/Release.gpg"
(
	cd "$site/apt"
	apt-ftparchive packages pool >dists/stable/main/binary-amd64/Packages
	gzip -9fk dists/stable/main/binary-amd64/Packages
	apt-ftparchive \
		-o APT::FTPArchive::Release::Origin=TuxShell \
		-o APT::FTPArchive::Release::Label=TuxShell \
		-o APT::FTPArchive::Release::Suite=stable \
		-o APT::FTPArchive::Release::Codename=stable \
		-o APT::FTPArchive::Release::Architectures=amd64 \
		-o APT::FTPArchive::Release::Components=main \
		-o "APT::FTPArchive::Release::Description=TuxShell packages" \
		release dists/stable >"$tmp"
	mv "$tmp" dists/stable/Release
)
gpg "${gpg_args[@]}" --clearsign -o "$dist/InRelease" "$dist/Release"
gpg "${gpg_args[@]}" --armor --detach-sign -o "$dist/Release.gpg" "$dist/Release"

# dnf: metadata, then its signature.
createrepo_c --update --quiet "$rpms"
gpg "${gpg_args[@]}" --armor --detach-sign -o "$rpms/repodata/repomd.xml.asc" "$rpms/repodata/repomd.xml"
cp -- "$here/tuxshell.repo" "$rpms/tuxshell.repo"

gpg --batch --yes --armor --export "$key" >"$site/tuxshell.asc"
echo "repository ready in $site: $(ls "$pool" | wc -l) .deb, $(ls "$rpms"/*.rpm | wc -l) .rpm"
