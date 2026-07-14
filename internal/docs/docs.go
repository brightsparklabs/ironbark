// Package docs surfaces product documentation embedded into the
// Ironbark binary so it can be displayed via the `ironbark docs`
// command without requiring network access (useful for
// airgapped deployments).
//
// The README itself lives at the repo root (`README.adoc`) and is
// outside the Go module. The Makefile's `build` target copies it into
// `src/resources/resources/README.adoc` immediately before `go build`
// so the existing `resources` package picks it up via its embedded
// filesystem.
//
// The copy is gitignored. If `go build` is invoked standalone (without
// running `make build` first) the README is simply absent from the
// embed and a short fallback message is returned instead, explaining
// to the reader how to produce a binary with the real README embedded.
package docs

import (
	"errors"
	"io/fs"

	"brightsparklabs.com/ironbark/resources"
)

// readMeResourcePath is the path (relative to the embedded resources
// directory) at which the Makefile drops the repo-root `README.adoc`.
const readMeResourcePath = "README.adoc"

// readMeFallback is returned by `ReadMe` when the README has not been
// embedded into the binary (typically because `go build` was invoked
// without running `make build` first).
const readMeFallback = `= Ironbark (README not embedded in this build)

This binary was built without the repository's README.adoc embedded into it.

View the README directly in the project repository:

* https://github.com/brightsparklabs/ironbark
`

// ReadMe returns the embedded README content. When the README is not
// present in the embedded resources (i.e. the binary was built without
// running ` + "`make build`" + `), a short fallback message is returned
// instead.
func ReadMe() string {
	data, err := resources.ReadFile(readMeResourcePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return readMeFallback
		}
		// Any other read error is rare and treated the same way -
		// returning a useful message is more user-friendly than
		// surfacing an opaque error from a documentation command.
		return readMeFallback
	}
	return string(data)
}
