// Test seams (design/TEST-SEAMS.10.md). zip.Writer.Create only fails
// once the writer is closed (or on a handful of internal-invariant
// conditions), and writing to an entry only fails on a genuine
// short/disk-full write to the underlying file — neither is reachable
// through the public Run() surface without controlling the zip
// machinery directly, so each gets a narrow seam.

package pack

import (
	"archive/zip"
	"io"
)

// zipCreate seams (*zip.Writer).Create for the entry-creation error
// arm in Run's file loop.
var zipCreate = func(zw *zip.Writer, name string) (io.Writer, error) {
	return zw.Create(name)
}

// zipWriteEntry seams the write of one file's bytes into its zip
// entry, for the write-failure arm in Run's file loop.
var zipWriteEntry = func(w io.Writer, data []byte) (int, error) {
	return w.Write(data)
}
