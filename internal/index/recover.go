package index

import (
	"errors"
	"fmt"
)

// errHandlerPanic marks a panic recovered at the asset-handler boundary. Third-
// party decoders (oov/psd, x/image/opentype) are not fuzz-hardened, so a
// malformed or hostile file can panic them; converting that panic into an error
// keeps one bad asset from crashing the whole scan.
var errHandlerPanic = errors.New("asset handler panicked")

// guard runs fn, recovering any panic into errHandlerPanic so the caller can log
// it and skip just this asset (or just this capability) instead of aborting the
// scan. Every call into a third-party handler method crosses this boundary.
func guard(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errHandlerPanic, r)
		}
	}()
	return fn()
}
