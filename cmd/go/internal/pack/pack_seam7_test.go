package pack

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestS7n_ZipCreateEntryError drives Run's zw.Create(f) failure arm
// by swapping the zipCreate seam.
func TestS7n_ZipCreateEntryError(t *testing.T) {
	orig := zipCreate
	t.Cleanup(func() { zipCreate = orig })
	zipCreate = func(zw *zip.Writer, name string) (io.Writer, error) {
		return nil, errors.New("boom-zip-create")
	}

	dir := writeModule(t, goodJsonic, map[string]string{"index.boru": "1 add 2"})
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "boom-zip-create") {
		t.Errorf("stderr = %q, want it to contain boom-zip-create", stderr.String())
	}
}

// TestS7n_ZipWriteEntryError drives Run's w.Write(data) failure arm
// by swapping the zipWriteEntry seam; zipCreate stays real so the
// entry writer itself is a genuine zip.Writer output.
func TestS7n_ZipWriteEntryError(t *testing.T) {
	orig := zipWriteEntry
	t.Cleanup(func() { zipWriteEntry = orig })
	zipWriteEntry = func(w io.Writer, data []byte) (int, error) {
		return 0, errors.New("boom-zip-write")
	}

	dir := writeModule(t, goodJsonic, map[string]string{"index.boru": "1 add 2"})
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("Run = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "boom-zip-write") {
		t.Errorf("stderr = %q, want it to contain boom-zip-write", stderr.String())
	}
}

// TestS7n_ZipCreateAndWriteEntryDefaultsWork pins that, with the
// seams left at their default (real) implementation, packing a
// module still produces a valid, readable zip — i.e. the seam
// indirection itself changes nothing observable.
func TestS7n_ZipCreateAndWriteEntryDefaultsWork(t *testing.T) {
	dir := writeModule(t, goodJsonic, map[string]string{"index.boru": "3 add 4"})
	var stdout, stderr bytes.Buffer
	code := Run([]string{dir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run = %d, want 0; stderr: %s", code, stderr.String())
	}
	zipPath := strings.TrimSpace(stdout.String())
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %s", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "index.boru" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry: %s", err)
		}
		defer rc.Close()
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read entry: %s", err)
		}
		if string(got) != "3 add 4" {
			t.Errorf("entry content = %q, want %q", got, "3 add 4")
		}
	}
}
