#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tdlib_prefix="${TDLIB_PREFIX:-${project_root}/.local/tdlib}"
version="${VERSION:-}"
tdlib_version="1.8.64"
tdlib_commit="49b3bcbb6bfebf2ed44dd9f25102d2e1a94a58c4"

if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]]; then
	printf 'VERSION must be a release tag such as v0.1.0 or v0.1.0-rc.1\n' >&2
	exit 1
fi
case "$(uname -s):$(uname -m)" in
	Linux:x86_64) ;;
	*)
		printf 'release packaging is currently supported only on Linux x86_64\n' >&2
		exit 1
		;;
esac

installed_commit="$(cat "${tdlib_prefix}/TDLIB_COMMIT" 2>/dev/null || true)"
installed_distribution="$(cat "${tdlib_prefix}/TDLIB_DISTRIBUTION" 2>/dev/null || true)"
if [[ "${installed_commit}" != "${tdlib_commit}" || "${installed_distribution}" != "prebuilt-tdlib@0.1008064.0" ]]; then
	printf 'release TDLib is not installed at %s; run make tdlib first\n' "${tdlib_prefix}" >&2
	exit 1
fi
library="${tdlib_prefix}/lib/libtdjson.so.${tdlib_version}"
if [[ ! -f "${library}" ]]; then
	printf 'missing TDLib shared library: %s\n' "${library}" >&2
	exit 1
fi

release_version="${version#v}"
release_name="telegram-tui_${release_version}_linux_x86_64"
dist_dir="${project_root}/dist"
stage_dir="${dist_dir}/${release_name}"
rm -rf "${dist_dir}"
install -d "${stage_dir}/lib" "${stage_dir}/share/licenses"

CGO_ENABLED=1 \
CGO_CFLAGS="-I${tdlib_prefix}/include" \
CGO_LDFLAGS="-Wl,-rpath,\$ORIGIN/lib -L${tdlib_prefix}/lib -ltdjson" \
go build -trimpath -tags 'tdlib libtdjson' \
	-ldflags "-s -w -X github.com/zylen-det/telegram-tui/internal/buildinfo.version=${version}" \
	-o "${stage_dir}/telegram-tui" "${project_root}/cmd/telegram-tui"

install -m 0755 "${library}" "${stage_dir}/lib/libtdjson.so.${tdlib_version}"
ln -s "libtdjson.so.${tdlib_version}" "${stage_dir}/lib/libtdjson.so"
install -m 0644 "${project_root}/LICENSE" "${stage_dir}/LICENSE"
install -m 0644 "${project_root}/README.md" "${stage_dir}/README.md"
install -m 0644 "${project_root}/third_party/tdlib/LICENSE_1_0.txt" "${stage_dir}/share/licenses/TDLib-LICENSE_1_0.txt"
install -m 0644 "${tdlib_prefix}/share/licenses/prebuilt-tdlib/LICENSE" "${stage_dir}/share/licenses/prebuilt-tdlib-LICENSE"

LD_LIBRARY_PATH="${tdlib_prefix}/lib${LD_LIBRARY_PATH:+:${LD_LIBRARY_PATH}}" \
GOFLAGS='-tags=tdlib,libtdjson' \
CGO_ENABLED=1 \
CGO_CFLAGS="-I${tdlib_prefix}/include" \
CGO_LDFLAGS="-L${tdlib_prefix}/lib -ltdjson" \
go run github.com/google/go-licenses/v2@v2.0.1 save "${project_root}/cmd/telegram-tui" \
	--save_path="${stage_dir}/share/licenses/go"

"${stage_dir}/telegram-tui" --version | grep -Fx "${version}" >/dev/null
LD_LIBRARY_PATH= ldd "${stage_dir}/telegram-tui" | grep -F "${stage_dir}/lib/libtdjson.so.${tdlib_version}" >/dev/null

archive="${dist_dir}/${release_name}.tar.gz"
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
	--use-compress-program='gzip -n' -C "${dist_dir}" -cf "${archive}" "${release_name}"
(
	cd "${dist_dir}"
	sha256sum "$(basename "${archive}")" >SHA256SUMS
)

printf 'Release artifacts written to %s\n' "${dist_dir}"
