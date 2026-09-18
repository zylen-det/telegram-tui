# Local go-tdlib patches

This directory is a minimal fork of
`github.com/zelenin/go-tdlib@v1.0.0-beta1.0.20260509025013-0dd3ea652682`.
It contains the upstream `LICENSE`, `go.mod`, and complete `client` package.
The pinned upstream module does not contain a `go.sum` file.

The fork carries three lifecycle fixes and one dynamic-linking adjustment:

1. Client options are applied synchronously, in declaration order, before the
   client is registered and its receiver starts. `WithProxy` records the
   request and performs it only after the receiver is available.
2. The cross-goroutine closed flag uses `atomic.Bool`.
3. An authorization handler error sends the TDLib close request and returns
   immediately, preserving both the handler and close errors with
   `errors.Join`. This avoids requesting another authorization state after the
   response channel has closed.
4. Dynamic `tdjson` builds link only to `libtdjson`; the shared library owns its
   native dependency declarations. This keeps the checksummed prebuilt release
   from acquiring unnecessary direct OpenSSL, zlib, and libstdc++ dependencies.

`client/client_lifecycle_test.go` exercises the patched behavior against the
real local TDLib without setting TDLib parameters or connecting to Telegram.
