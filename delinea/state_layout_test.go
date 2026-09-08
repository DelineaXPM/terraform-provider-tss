package delinea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEncryptedStateLayout(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		metadata  string
		workspace string
		wantError string
	}{
		{name: "implicit local"},
		{name: "explicit defaults", config: `terraform {
  backend "local" {
    path          = "./terraform.tfstate"
    workspace_dir = "terraform.tfstate.d"
  }
}`},
		{name: "remote configured", config: `terraform {
  backend "s3" {}
}`, wantError: `configured backend is "s3"`},
		{name: "cloud configured", config: `terraform {
  cloud {}
}`, wantError: "configures Terraform cloud state"},
		{name: "custom path configured", config: `terraform {
  backend "local" {
    path = "state/custom.tfstate"
  }
}`, wantError: "local backend path"},
		{name: "remote initialized", metadata: `{"backend":{"type":"remote","config":{}}}`, wantError: `initialized backend is "remote"`},
		{name: "custom path initialized", metadata: `{"backend":{"type":"local","config":{"path":"state/custom.tfstate","workspace_dir":null}}}`, wantError: "initialized local backend path"},
		{name: "named workspace", workspace: "production", wantError: `selected workspace is "production"`},
		{name: "malformed metadata", metadata: `{`, wantError: "parse initialized Terraform backend metadata"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workDir := t.TempDir()
			dataDir := filepath.Join(workDir, ".terraform")
			if test.config != "" {
				if err := os.WriteFile(filepath.Join(workDir, "main.tf"), []byte(test.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if test.metadata != "" {
				if err := os.Mkdir(dataDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dataDir, "terraform.tfstate"), []byte(test.metadata), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err := validateEncryptedStateLayout(workDir, dataDir, test.workspace)
			if test.wantError == "" && err != nil {
				t.Fatalf("validateEncryptedStateLayout() error = %v", err)
			}
			if test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError)) {
				t.Fatalf("validateEncryptedStateLayout() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

func TestValidateEncryptedStateLayoutReadsWorkspaceFile(t *testing.T) {
	workDir := t.TempDir()
	dataDir := filepath.Join(workDir, ".terraform")
	if err := os.Mkdir(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "environment"), []byte("staging\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateEncryptedStateLayout(workDir, dataDir, ""); err == nil || !strings.Contains(err.Error(), `selected workspace is "staging"`) {
		t.Fatalf("validateEncryptedStateLayout() error = %v", err)
	}
}
