package picker

import (
	"io"
	"os"
)

// stderr is the output stream Bubble Tea writes to.
func stderr() io.Writer { return os.Stderr }
