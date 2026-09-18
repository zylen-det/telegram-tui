#!/usr/bin/env bash
set -euo pipefail

# Arch Linux prerequisites: base-devel cmake gperf openssl zlib.
for tool in git cmake gperf; do
	if ! command -v "${tool}" >/dev/null 2>&1; then
		printf 'missing required tool: %s\n' "${tool}" >&2
		printf 'Arch prerequisites: base-devel cmake gperf openssl zlib\n' >&2
		exit 1
	fi
done

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source_dir="${project_root}/.local/src/tdlib"
build_dir="${source_dir}/build"
install_dir="${TDLIB_PREFIX:-${project_root}/.local/tdlib}"
commit="49b3bcbb6bfebf2ed44dd9f25102d2e1a94a58c4"

mkdir -p "$(dirname "${source_dir}")" "${install_dir}"
if [[ ! -d "${source_dir}/.git" ]]; then
	git clone https://github.com/tdlib/td.git "${source_dir}"
fi
git -C "${source_dir}" fetch --depth 1 origin "${commit}"
git -C "${source_dir}" checkout --detach "${commit}"

cmake -S "${source_dir}" -B "${build_dir}" \
	-DCMAKE_BUILD_TYPE=Release \
	-DCMAKE_INSTALL_PREFIX="${install_dir}" \
	-DCMAKE_INSTALL_LIBDIR=lib
cmake --build "${build_dir}" --target install --parallel
printf '%s\n' "${commit}" >"${install_dir}/TDLIB_COMMIT"
printf 'source\n' >"${install_dir}/TDLIB_DISTRIBUTION"

printf 'TDLib installed at %s\n' "${install_dir}"
