#!/bin/sh
set -eu

repository="${TELEGRAM_TUI_REPOSITORY:-zylen-det/telegram-tui}"
prefix="${PREFIX:-${HOME:-}/.local}"
version="${VERSION:-}"

fail() {
	printf 'telegram-tui installer: %s\n' "$*" >&2
	exit 1
}

[ -n "${prefix}" ] || fail 'HOME is unset; provide PREFIX explicitly'

case "$(uname -s):$(uname -m)" in
	Linux:x86_64) ;;
	*) fail 'only Linux x86-64 release archives are currently available' ;;
esac

for tool in awk cp curl grep ln mkdir mktemp mv rm sha256sum tar uname; do
	command -v "${tool}" >/dev/null 2>&1 || fail "missing required tool: ${tool}"
done

if [ -z "${version}" ]; then
	latest_url="https://github.com/${repository}/releases/latest"
	effective_url="$(curl --fail --location --silent --show-error --output /dev/null --write-out '%{url_effective}' "${latest_url}")" || fail 'could not resolve the latest release'
	version="${effective_url##*/}"
else
	case "${version}" in
		v*) ;;
		*) version="v${version}" ;;
	esac
fi

printf '%s\n' "${version}" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$' || fail "invalid release version: ${version}"

release_version="${version#v}"
release_name="telegram-tui_${release_version}_linux_x86_64"
archive_name="${release_name}.tar.gz"
release_url="${TELEGRAM_TUI_RELEASE_URL:-https://github.com/${repository}/releases/download/${version}}"

temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/telegram-tui-install.XXXXXX")"
cleanup() {
	rm -rf "${temporary_dir}"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

archive="${temporary_dir}/${archive_name}"
checksums="${temporary_dir}/SHA256SUMS"
curl --fail --location --silent --show-error "${release_url}/${archive_name}" --output "${archive}" || fail "could not download ${archive_name}"
curl --fail --location --silent --show-error "${release_url}/SHA256SUMS" --output "${checksums}" || fail 'could not download SHA256SUMS'

checksum="$(awk -v name="${archive_name}" '$2 == name { print $1; exit }' "${checksums}")"
case "${checksum}" in
	????????????????????????????????????????????????????????????????)
		printf '%s\n' "${checksum}" | grep -Eq '^[0-9A-Fa-f]{64}$' || fail 'release checksum is invalid'
		;;
	*) fail "SHA256SUMS does not contain ${archive_name}" ;;
esac
printf '%s  %s\n' "${checksum}" "${archive}" | sha256sum --check --status || fail 'release archive checksum mismatch'

tar -xzf "${archive}" -C "${temporary_dir}"
source_dir="${temporary_dir}/${release_name}"
[ -x "${source_dir}/telegram-tui" ] || fail 'release archive does not contain the executable'
[ -f "${source_dir}/lib/libtdjson.so.1.8.64" ] || fail 'release archive does not contain the pinned TDLib library'
installed_version="$("${source_dir}/telegram-tui" --version)" || fail 'downloaded executable could not start'
[ "${installed_version}" = "${version}" ] || fail "downloaded version is ${installed_version}, expected ${version}"

install_root="${prefix}/opt/telegram-tui"
version_dir="${install_root}/${version}"
candidate_dir="${install_root}/.${version}.new.$$"
mkdir -p "${install_root}" "${prefix}/bin"
rm -rf "${candidate_dir}"
mkdir -p "${candidate_dir}"
cp -a "${source_dir}/." "${candidate_dir}/"
rm -rf "${version_dir}"
mv "${candidate_dir}" "${version_dir}"
ln -sfn "../opt/telegram-tui/${version}/telegram-tui" "${prefix}/bin/telegram-tui"

printf 'telegram-tui %s installed at %s\n' "${version}" "${version_dir}"
printf 'command: %s/bin/telegram-tui\n' "${prefix}"
case ":${PATH:-}:" in
	*":${prefix}/bin:"*) ;;
	*) printf 'add %s/bin to PATH to run telegram-tui directly\n' "${prefix}" >&2 ;;
esac
