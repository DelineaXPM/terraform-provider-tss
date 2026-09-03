package delinea

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEncryptDecryptFileTightensPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}

	path := filepath.Join(t.TempDir(), "terraform.tfstate")
	want := []byte(`{"secret":"value"}`)
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EncryptFile("passphrase", path); err != nil {
		t.Fatal(err)
	}
	assertStateFileMode(t, path)

	if err := DecryptFile("passphrase", path); err != nil {
		t.Fatal(err)
	}
	assertStateFileMode(t, path)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("decrypted content = %q, want %q", got, want)
	}
}

func TestDecryptMissingFileCreatesProtectedPlaceholder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions are verified by the DACL test")
	}

	path := filepath.Join(t.TempDir(), "terraform.tfstate")
	if err := DecryptFile("passphrase", path); err != nil {
		t.Fatal(err)
	}
	assertStateFileMode(t, path)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 0 {
		t.Fatalf("placeholder content = %q, want empty", contents)
	}
}

func TestEncryptFileReportsUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "terraform.tfstate")
	if err := os.WriteFile(path, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := EncryptFile("passphrase", path); err == nil {
		t.Fatal("encrypting an unreadable state file succeeded")
	}
}

func TestWriteStateFileFailurePreservesOriginal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix directory permission bits")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "terraform.tfstate")
	want := []byte("original")
	if err := os.WriteFile(path, want, stateFileMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := writeStateFile(path, []byte("replacement")); err == nil {
		t.Fatal("writeStateFile succeeded without directory write permission")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("original content = %q, want %q", got, want)
	}
}

func assertStateFileMode(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != stateFileMode {
		t.Fatalf("state file mode = %04o, want %04o", got, stateFileMode)
	}
}
