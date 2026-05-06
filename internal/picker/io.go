package picker

import (
	"io"
	"os"
)

// stderr is the output stream Bubble Tea writes to. Split into its own
// helper so tests can override if they ever need to render headlessly.
func stderr() io.Writer { return os.Stderr }
