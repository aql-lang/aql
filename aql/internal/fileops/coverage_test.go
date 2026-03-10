package fileops

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOSFileOpsResolvePathGetwdError triggers the os.Getwd error path
// by removing the current working directory temporarily.
func TestOSFileOpsResolvePathGetwdError(t *testing.T) {
	o := &OSFileOps{}

	// Create a temporary directory, cd into it, then remove it.
	tmpDir, err := os.MkdirTemp("", "fileops-test-*")
	if err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir) //nolint:errcheck

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Remove the directory while we're in it, making Getwd fail.
	if err := os.Remove(tmpDir); err != nil {
		t.Fatal(err)
	}

	_, err = o.ResolvePath("relative.txt")
	if err == nil {
		t.Fatal("expected error from ResolvePath when cwd is removed")
	}
}

// TestOSFileOpsReadFileResolveError triggers the ResolvePath error path
// in ReadFile by the same cwd-removal trick.
func TestOSFileOpsReadFileResolveError(t *testing.T) {
	o := &OSFileOps{}

	tmpDir, err := os.MkdirTemp("", "fileops-test-*")
	if err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir) //nolint:errcheck

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(tmpDir); err != nil {
		t.Fatal(err)
	}

	_, err = o.ReadFile("relative.txt")
	if err == nil {
		t.Fatal("expected error from ReadFile when cwd is removed")
	}
}

// TestOSFileOpsWriteFileResolveError triggers the ResolvePath error path
// in WriteFile by the same cwd-removal trick.
func TestOSFileOpsWriteFileResolveError(t *testing.T) {
	o := &OSFileOps{}

	tmpDir, err := os.MkdirTemp("", "fileops-test-*")
	if err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir) //nolint:errcheck

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(tmpDir); err != nil {
		t.Fatal(err)
	}

	err = o.WriteFile("relative.txt", []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error from WriteFile when cwd is removed")
	}
}

// TestOSFileOpsWriteMkdirAllError triggers the MkdirAll error path
// by writing to a path under /proc which is a special filesystem.
func TestOSFileOpsWriteMkdirAllError(t *testing.T) {
	o := &OSFileOps{}
	// /proc/1/fdinfo is not writable even as root
	err := o.WriteFile("/proc/1/fdinfo/newsubdir/file.txt", []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error from WriteFile due to MkdirAll failure")
	}
}

// TestOSFileOpsWriteFileError triggers the os.WriteFile error
// by writing to a path that is a directory.
func TestOSFileOpsWriteFileError(t *testing.T) {
	o := &OSFileOps{}
	tmpDir := t.TempDir()

	// Create a directory where WriteFile expects a file.
	dirAsFile := filepath.Join(tmpDir, "actuallyADir")
	if err := os.Mkdir(dirAsFile, 0755); err != nil {
		t.Fatal(err)
	}

	err := o.WriteFile(dirAsFile, []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error writing to a directory path")
	}
}

// TestMemFileOpsResolvePathAbsolute tests MemFileOps.ResolvePath with an absolute path.
func TestMemFileOpsResolvePathAbsolute(t *testing.T) {
	m := NewMem()
	got, err := m.ResolvePath("/absolute/path.txt")
	if err != nil {
		t.Fatalf("ResolvePath error: %v", err)
	}
	if got != "/absolute/path.txt" {
		t.Errorf("got %q, want %q", got, "/absolute/path.txt")
	}
}

// TestMemFileOpsReadWriteAbsolutePath tests read/write with absolute paths.
func TestMemFileOpsReadWriteAbsolutePath(t *testing.T) {
	m := NewMem()
	path := "/some/abs/path.txt"
	data := []byte("absolute data")

	if err := m.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	got, err := m.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "absolute data" {
		t.Errorf("got %q, want %q", got, "absolute data")
	}
}

// TestMemFileOpsWriteEmptyData tests writing empty data.
func TestMemFileOpsWriteEmptyData(t *testing.T) {
	m := NewMem()
	if err := m.WriteFile("empty.txt", []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	got, err := m.ReadFile("empty.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty data, got %q", got)
	}
}
