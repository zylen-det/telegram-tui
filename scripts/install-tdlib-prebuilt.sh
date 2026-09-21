#!/usr/bin/env bash
set -euo pipefail

# Prebuilt TDLib 1.8.64, built from the commit required by the pinned Go
# wrapper. The package is produced by eilvelia/tdl's public GitHub Actions
# workflow and published to npm with provenance.
tdlib_version="1.8.64"
tdlib_commit="49b3bcbb6bfebf2ed44dd9f25102d2e1a94a58c4"
package_version="0.1008064.0"
package_url="https://registry.npmjs.org/@prebuilt-tdlib/linux-x64-glibc/-/linux-x64-glibc-${package_version}.tgz"
package_sha256="06a9f9b6ca8eee13600a5855c573bf318515d8550f4776dd2e049d71d28c59bf"
library_sha256="45430e64ea76abd56390beb4bd4ce354ff5c429bd273433b11ffc3ba318df4a7"

case "$(uname -s):$(uname -m)" in
	Linux:x86_64) ;;
	*)
		printf 'prebuilt TDLib is available only for Linux x86_64; use make tdlib-source on this platform\n' >&2
		exit 1
		;;
esac

for tool in curl tar sha256sum install; do
	if ! command -v "${tool}" >/dev/null 2>&1; then
		printf 'missing required tool: %s\n' "${tool}" >&2
		exit 1
	fi
done

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
install_dir="${TDLIB_PREFIX:-${project_root}/.local/tdlib}"
headers_dir="${project_root}/third_party/tdlib/include"
license_file="${project_root}/third_party/tdlib/LICENSE_1_0.txt"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "${temporary_dir}"' EXIT

archive="${temporary_dir}/tdlib.tgz"
curl --fail --location --silent --show-error "${package_url}" --output "${archive}"
printf '%s  %s\n' "${package_sha256}" "${archive}" | sha256sum --check --status || {
	printf 'prebuilt TDLib archive checksum mismatch\n' >&2
	exit 1
}
tar -xzf "${archive}" -C "${temporary_dir}" package/libtdjson.so package/LICENSE
printf '%s  %s\n' "${library_sha256}" "${temporary_dir}/package/libtdjson.so" | sha256sum --check --status || {
	printf 'prebuilt TDLib library checksum mismatch\n' >&2
	exit 1
}

install -d \
	"${install_dir}/lib" \
	"${install_dir}/include/td/telegram" \
	"${install_dir}/share/licenses/prebuilt-tdlib" \
	"${install_dir}/share/licenses/tdlib"
install -m 0755 "${temporary_dir}/package/libtdjson.so" "${install_dir}/lib/libtdjson.so.${tdlib_version}"
ln -sfn "libtdjson.so.${tdlib_version}" "${install_dir}/lib/libtdjson.so"
install -m 0644 "${headers_dir}/td/telegram/td_json_client.h" "${install_dir}/include/td/telegram/td_json_client.h"
install -m 0644 "${headers_dir}/td/telegram/tdjson_export.h" "${install_dir}/include/td/telegram/tdjson_export.h"
install -m 0644 "${temporary_dir}/package/LICENSE" "${install_dir}/share/licenses/prebuilt-tdlib/LICENSE"
install -m 0644 "${license_file}" "${install_dir}/share/licenses/tdlib/LICENSE_1_0.txt"
printf '%s\n' "${tdlib_commit}" >"${install_dir}/TDLIB_COMMIT"
printf 'prebuilt-tdlib@%s\n' "${package_version}" >"${install_dir}/TDLIB_DISTRIBUTION"

printf 'TDLib %s (%s) installed at %s\n' "${tdlib_version}" "${tdlib_commit}" "${install_dir}"
