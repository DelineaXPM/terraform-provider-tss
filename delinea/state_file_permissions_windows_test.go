//go:build windows

package delinea

import (
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestStateFilesUseProtectedCurrentUserDACL(t *testing.T) {
	tests := map[string]func(string) error{
		"write": func(path string) error {
			return writeStateFile(path, []byte("secret state"))
		},
		"missing decrypt": func(path string) error {
			return DecryptFile("passphrase", path)
		},
	}
	for name, create := range tests {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir() + `\terraform.tfstate`
			if err := create(path); err != nil {
				t.Fatal(err)
			}
			assertProtectedCurrentUserDACL(t, path)
			if name == "missing decrypt" {
				contents, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if len(contents) != 0 {
					t.Fatalf("placeholder content = %q, want empty", contents)
				}
			}
		})
	}
}

func assertProtectedCurrentUserDACL(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("state file DACL inherits permissions")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("state file DACL ACE count = %v, want 1", dacl)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		t.Fatalf("state file DACL ACE type = %d, want allow", ace.Header.AceType)
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.Equals(user.User.Sid) {
		t.Fatalf("state file DACL %q does not grant the current user", descriptor.String())
	}
}
