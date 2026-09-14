package delinea

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLinuxInitWrapperBootstrapsWithEncryptedStateAndForwardsArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash wrapper is not used on Windows")
	}

	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	providerSource := filepath.Join(workDir, "provider-source")
	providerScript := `#!/bin/sh
[ "$1" = "state-helper-version" ] || [ "$1" = "state-helper-check-layout" ] || [ "$TFSTATE_PASSPHRASE" = "test-passphrase" ] || exit 3
case "$1" in
state-helper-version)
    printf '1\n'
    ;;
state-helper-check-layout)
    ;;
decrypt)
    if [ -e "$2" ]; then
        printf '{"version":4}' > "$2"
    fi
    ;;
encrypt)
    if [ "$FAIL_PRIMARY_ENCRYPT" = "1" ] && [ "$2" = "terraform.tfstate" ]; then
        exit 4
    fi
    if [ -e "$2" ]; then
        printf 'encrypted' > "$2"
    fi
    ;;
*)
    exit 2
    ;;
esac
`
	if err := os.WriteFile(providerSource, []byte(providerScript), 0o700); err != nil {
		t.Fatal(err)
	}
	terraformPath := filepath.Join(binDir, "terraform")
	terraformScript := `#!/bin/sh
[ -z "$TFSTATE_PASSPHRASE" ] || exit 40
if [ "$1:$2" = "version:-json" ]; then
    printf '{"platform":"test_arch"}\n'
    exit 0
fi
printf '%s\n' "$*" >> "$CALL_LOG"
if [ "$1" = "init" ] && [ "$2" = "-backend=false" ]; then
    if [ -e terraform.tfstate ]; then
        exit 41
    fi
    provider_dir=.terraform/providers/registry.terraform.io/delineaxpm/tss/5.0.0/test_arch
    mkdir -p "$provider_dir" || exit 42
    cp "$FAKE_PROVIDER_SOURCE" "$provider_dir/terraform-provider-tss_v5.0.0" || exit 43
    chmod 700 "$provider_dir/terraform-provider-tss_v5.0.0" || exit 44
    cat > .terraform.lock.hcl <<'LOCKFILE'
provider "registry.terraform.io/delineaxpm/tss" {
  version = "5.0.0"
}
LOCKFILE
    exit 0
fi
if [ "$(cat terraform.tfstate)" != '{"version":4}' ]; then
    exit 45
fi
`
	if err := os.WriteFile(terraformPath, []byte(terraformScript), 0o700); err != nil {
		t.Fatal(err)
	}
	decoyPath := filepath.Join(workDir, "terraform-provider-tss")
	if err := os.WriteFile(decoyPath, []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	legacyProviderDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "4.0.0", "test_arch")
	if err := os.MkdirAll(legacyProviderDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyProviderDir, "terraform-provider-tss_v4.0.0"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte(`provider "registry.terraform.io/delineaxpm/tss" {
  version = "4.0.0"
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte("encrypted"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	callLog := filepath.Join(workDir, "calls.log")
	scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "linux", "terraform_init.sh"))
	if err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(workDir, "plugins with spaces")
	cmd := exec.Command("bash", scriptPath, "--upgrade", "-reconfigure", "--plugin-dir", pluginDir)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+workDir,
		"TFSTATE_PASSPHRASE=test-passphrase",
		`TF_CLI_ARGS=-var="note -state=not-an-option"`,
		"CALL_LOG="+callLog,
		"FAKE_PROVIDER_SOURCE="+providerSource,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper failed: %v\n%s", err, output)
	}
	calls, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(calls)), "init -backend=false --upgrade --plugin-dir "+pluginDir+"\ninit --upgrade -reconfigure --plugin-dir "+pluginDir; got != want {
		t.Fatalf("terraform calls = %q, want %q", got, want)
	}
	for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
		contents, err := os.ReadFile(filepath.Join(workDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(contents) != "encrypted" {
			t.Fatalf("%s contents = %q, want encrypted", name, contents)
		}
	}
	if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock directory remains after wrapper exit: %v", err)
	}
	staleProviderDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "4.0.0", "test_arch")
	if err := os.MkdirAll(staleProviderDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleProviderDir, "terraform-provider-tss_v4.0.0"), []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	failingCmd := exec.Command("bash", scriptPath)
	failingCmd.Dir = workDir
	failingCmd.Env = append(cmd.Env, "FAIL_PRIMARY_ENCRYPT=1")
	output, err = failingCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("wrapper succeeded despite encryption failure:\n%s", output)
	}
	if !strings.Contains(string(output), "retaining lockfile.lock") {
		t.Fatalf("wrapper did not report retained lock:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); err != nil {
		t.Fatalf("lock directory was not retained after encryption failure: %v", err)
	}
}

func TestLinuxInitWrapperRejectsFromModuleBeforeLocking(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash wrapper is not used on Windows")
	}
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	callLog := filepath.Join(workDir, "calls.log")
	terraformPath := filepath.Join(binDir, "terraform")
	if err := os.WriteFile(terraformPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$CALL_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "linux", "terraform_init.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath, "-from-module=../source-module")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TFSTATE_PASSPHRASE=test-passphrase",
		"CALL_LOG="+callLog,
	)
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "does not support -from-module") {
		t.Fatalf("wrapper result = %v\n%s", err, output)
	}
	if _, err := os.Stat(callLog); !os.IsNotExist(err) {
		t.Fatalf("terraform was invoked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock directory was created: %v", err)
	}
}

func TestLinuxWrappersRejectStateRedirectionBeforeLocking(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash wrappers are not used on Windows")
	}
	tests := []struct {
		operation string
		argument  string
		env       string
		message   string
	}{
		{operation: "init", argument: "-backend-config=backend.hcl", message: "does not support -backend-config"},
		{operation: "init", argument: "--backend-config=backend.hcl", message: "does not support -backend-config"},
		{operation: "init", argument: "--from-module=../source-module", message: "does not support -from-module"},
		{operation: "apply", argument: "-state=alternate.tfstate", message: "does not support -state"},
		{operation: "apply", argument: "--state=alternate.tfstate", message: "does not support -state"},
		{operation: "destroy", argument: "-backup", message: "does not support -state"},
		{operation: "destroy", argument: "--backup", message: "does not support -state"},
		{operation: "apply", env: "TF_CLI_ARGS_apply=-state-out=alternate.tfstate", message: "redirects state"},
		{operation: "apply", env: "TF_CLI_ARGS_apply=--state-out=alternate.tfstate", message: "redirects state"},
		{operation: "apply", env: `TF_CLI_ARGS_apply=--sta"te"=alternate.tfstate`, message: "redirects state"},
		{operation: "destroy", env: `TF_CLI_ARGS_destroy=-sta\te-out=alternate.tfstate`, message: "redirects state"},
		{operation: "init", env: "TF_CLI_ARGS_init=--backend-config=backend.hcl", message: "does not support"},
		{operation: "init", env: `TF_CLI_ARGS_init=--backend-"config"=backend.hcl`, message: "does not support"},
		{operation: "init", env: "TF_CLI_ARGS_init=--from-module=../source-module", message: "does not support"},
	}
	for _, test := range tests {
		t.Run(test.operation+"/"+test.argument+test.env, func(t *testing.T) {
			workDir := t.TempDir()
			scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "linux", "terraform_"+test.operation+".sh"))
			if err != nil {
				t.Fatal(err)
			}
			arguments := []string{}
			if test.argument != "" {
				arguments = append(arguments, test.argument)
			}
			cmd := exec.Command("bash", append([]string{scriptPath}, arguments...)...)
			cmd.Dir = workDir
			cmd.Env = append(environmentWithout("TFSTATE_PASSPHRASE", "TF_CLI_ARGS", "TF_CLI_ARGS_init", "TF_CLI_ARGS_apply", "TF_CLI_ARGS_destroy"), "TFSTATE_PASSPHRASE=test-passphrase")
			if test.env != "" {
				cmd.Env = append(cmd.Env, test.env)
			}
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.message) {
				t.Fatalf("wrapper result = %v\n%s", err, output)
			}
			if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); !os.IsNotExist(err) {
				t.Fatalf("lock directory was created: %v", err)
			}
		})
	}
}

func TestLinuxApplyRejectsUnsupportedLayoutBeforeDecrypting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash wrapper is not used on Windows")
	}
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	providerDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "5.0.0", "test_arch")
	if err := os.MkdirAll(providerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	providerScript := `#!/bin/sh
case "$1" in
state-helper-version) printf '1\n' ;;
state-helper-check-layout) printf 'custom backend path is unsupported\n' >&2; exit 7 ;;
encrypt|decrypt) printf changed > "$2" ;;
*) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(providerDir, "terraform-provider-tss_v5.0.0"), []byte(providerScript), 0o700); err != nil {
		t.Fatal(err)
	}
	terraformScript := `#!/bin/sh
if [ "$1:$2" = "version:-json" ]; then
    printf '{"platform":"test_arch"}\n'
    exit 0
fi
printf ran > terraform-ran
`
	if err := os.WriteFile(filepath.Join(binDir, "terraform"), []byte(terraformScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte(`provider "registry.terraform.io/delineaxpm/tss" {
  version = "5.0.0"
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte("ciphertext"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "linux", "terraform_apply.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	cmd.Env = append(environmentWithout("TFSTATE_PASSPHRASE", "TF_CLI_ARGS", "TF_CLI_ARGS_apply"),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TFSTATE_PASSPHRASE=test-passphrase",
	)
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "No state was decrypted") {
		t.Fatalf("wrapper result = %v\n%s", err, output)
	}
	for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
		contents, err := os.ReadFile(filepath.Join(workDir, name))
		if err != nil || string(contents) != "ciphertext" {
			t.Fatalf("%s contents=%q err=%v", name, contents, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock directory was created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "terraform-ran")); !os.IsNotExist(err) {
		t.Fatalf("terraform ran after layout rejection: %v", err)
	}
}

func TestLinuxStateWrappersUseLockfileVersionWithStaleInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash wrappers are not used on Windows")
	}

	for _, operation := range []string{"apply", "destroy"} {
		t.Run(operation, func(t *testing.T) {
			workDir := t.TempDir()
			binDir := filepath.Join(workDir, "bin")
			if err := os.Mkdir(binDir, 0o700); err != nil {
				t.Fatal(err)
			}
			currentProviderDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "5.0.0", "test_arch")
			staleProviderDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "4.0.0", "test_arch")
			foreignProviderDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "5.0.0", "foreign_arch")
			if err := os.MkdirAll(currentProviderDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(staleProviderDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(foreignProviderDir, 0o700); err != nil {
				t.Fatal(err)
			}
			providerScript := `#!/bin/sh
[ "$1" = "state-helper-version" ] || [ "$1" = "state-helper-check-layout" ] || [ "$TFSTATE_PASSPHRASE" = "test-passphrase" ] || exit 3
case "$1" in
state-helper-version) printf '1\n' ;;
state-helper-check-layout) ;;
decrypt) printf 'decrypted' > "$2" ;;
encrypt) printf 'encrypted' > "$2" ;;
*) exit 2 ;;
esac
`
			if err := os.WriteFile(filepath.Join(currentProviderDir, "terraform-provider-tss_v5.0.0"), []byte(providerScript), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(staleProviderDir, "terraform-provider-tss_v4.0.0"), []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(foreignProviderDir, "terraform-provider-tss_v5.0.0"), []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			lockFile := `provider "registry.terraform.io/delineaxpm/tss" {
  version = "5.0.0"
}
`
			if err := os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte(lockFile), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
				if err := os.WriteFile(filepath.Join(workDir, name), []byte("encrypted"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			terraformPath := filepath.Join(binDir, "terraform")
			terraformScript := `#!/bin/sh
[ -z "$TFSTATE_PASSPHRASE" ] || exit 40
if [ "$1:$2" = "version:-json" ]; then
    printf '{"platform":"test_arch"}\n'
    exit 0
fi
[ "$1" = "$EXPECTED_OPERATION" ] || exit 41
[ "$(cat terraform.tfstate)" = "decrypted" ] || exit 42
`
			if err := os.WriteFile(terraformPath, []byte(terraformScript), 0o700); err != nil {
				t.Fatal(err)
			}
			scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "linux", "terraform_"+operation+".sh"))
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", scriptPath)
			cmd.Dir = workDir
			cmd.Env = append(os.Environ(),
				"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"TFSTATE_PASSPHRASE=test-passphrase",
				"EXPECTED_OPERATION="+operation,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("wrapper failed: %v\n%s", err, output)
			}
			for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
				contents, err := os.ReadFile(filepath.Join(workDir, name))
				if err != nil {
					t.Fatal(err)
				}
				if string(contents) != "encrypted" {
					t.Fatalf("%s contents = %q, want encrypted", name, contents)
				}
			}
		})
	}
}

func TestLinuxApplyRejectsLegacyHelperBeforeChangingState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash wrapper is not used on Windows")
	}

	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	providerDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "4.0.0", "test_arch")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyProvider := `#!/bin/sh
case "$1" in
encrypt|decrypt) exit 0 ;;
*) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(providerDir, "terraform-provider-tss_v4.0.0"), []byte(legacyProvider), 0o700); err != nil {
		t.Fatal(err)
	}
	terraformScript := `#!/bin/sh
if [ "$1:$2" = "version:-json" ]; then
    printf '{"platform":"test_arch"}\n'
    exit 0
fi
printf ran > terraform-ran
`
	if err := os.WriteFile(filepath.Join(binDir, "terraform"), []byte(terraformScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte(`provider "registry.terraform.io/delineaxpm/tss" {
  version = "4.0.0"
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte("ciphertext"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "linux", "terraform_apply.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	cmd.Env = append(environmentWithout("TFSTATE_PASSPHRASE", "TF_VAR_tss_username", "TF_VAR_tss_password"),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TFSTATE_PASSPHRASE=test-passphrase",
	)
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "state-helper protocol 1") {
		t.Fatalf("wrapper result err=%v output=%s", err, output)
	}
	for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
		contents, readErr := os.ReadFile(filepath.Join(workDir, name))
		if readErr != nil || string(contents) != "ciphertext" {
			t.Fatalf("%s contents=%q err=%v", name, contents, readErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "terraform-ran")); !os.IsNotExist(statErr) {
		t.Fatalf("terraform ran after legacy helper rejection: %v", statErr)
	}
}

func TestLinuxStateWrappersRejectPartialLegacyCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash wrappers are not used on Windows")
	}

	baseEnv := environmentWithout("TFSTATE_PASSPHRASE", "TF_VAR_tss_username", "TF_VAR_tss_password", "STATE_PASSPHRASE")
	for _, operation := range []string{"init", "apply", "destroy"} {
		for _, partial := range []string{"TF_VAR_tss_username=user", "TF_VAR_tss_password=password"} {
			t.Run(operation+"/"+strings.SplitN(partial, "=", 2)[0], func(t *testing.T) {
				scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "linux", "terraform_"+operation+".sh"))
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("bash", scriptPath)
				cmd.Dir = t.TempDir()
				cmd.Env = append(baseEnv, partial)
				output, err := cmd.CombinedOutput()
				if err == nil || !strings.Contains(string(output), "TF_VAR_tss_username and TF_VAR_tss_password") {
					t.Fatalf("wrapper result err=%v output=%s", err, output)
				}
			})
		}
	}
}

func TestLinuxStateWrappersRejectUnsafeLockfileVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash wrappers are not used on Windows")
	}

	for _, operation := range []string{"init", "apply", "destroy"} {
		t.Run(operation, func(t *testing.T) {
			workDir := t.TempDir()
			binDir := filepath.Join(workDir, "bin")
			providerRoot := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss")
			escapedRoot := filepath.Join(workDir, "escaped")
			if err := os.MkdirAll(filepath.Join(escapedRoot, "test_arch"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(providerRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(binDir, 0o700); err != nil {
				t.Fatal(err)
			}
			lockedVersion, err := filepath.Rel(providerRoot, escapedRoot)
			if err != nil {
				t.Fatal(err)
			}
			lockFile := "provider \"registry.terraform.io/delineaxpm/tss\" {\n  version = \"" + filepath.ToSlash(lockedVersion) + "\"\n}\n"
			if err := os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte(lockFile), 0o600); err != nil {
				t.Fatal(err)
			}
			providerPath := filepath.Join(escapedRoot, "test_arch", "terraform-provider-tss_v5.0.0")
			if err := os.WriteFile(providerPath, []byte("#!/bin/sh\nprintf invoked > \"$MARKER\"\nexit 1\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			terraformPath := filepath.Join(binDir, "terraform")
			if err := os.WriteFile(terraformPath, []byte("#!/bin/sh\n[ \"$1:$2\" = \"version:-json\" ] && printf '{\"platform\":\"test_arch\"}\\n'\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "linux", "terraform_"+operation+".sh"))
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(workDir, "provider-invoked")
			cmd := exec.Command("bash", scriptPath)
			cmd.Dir = workDir
			cmd.Env = append(environmentWithout("TFSTATE_PASSPHRASE", "TF_VAR_tss_username", "TF_VAR_tss_password"),
				"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"TFSTATE_PASSPHRASE=test-passphrase",
				"MARKER="+marker,
			)
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "provider version in .terraform.lock.hcl is invalid") {
				t.Fatalf("wrapper result err=%v output=%s", err, output)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("escaped provider was invoked: %v", err)
			}
			if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); !os.IsNotExist(err) {
				t.Fatalf("lock directory was created: %v", err)
			}
		})
	}
}

func TestLinuxApplyRetainsLockWhenBackupDecryptFailsAfterPrimary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash wrapper is not used on Windows")
	}

	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	providerDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "5.0.0", "test_arch")
	if err := os.MkdirAll(providerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(providerDir, "terraform-provider-tss_v5.0.0")
	providerScript := `#!/bin/sh
case "$1:$2" in
state-helper-version:) printf '1\n' ;;
state-helper-check-layout:) ;;
decrypt:terraform.tfstate) printf 'decrypted' > "$2" ;;
decrypt:terraform.tfstate.backup) exit 9 ;;
encrypt:terraform.tfstate) printf 'encrypted' > "$2" ;;
*) exit 8 ;;
esac
`
	if err := os.WriteFile(providerPath, []byte(providerScript), 0o700); err != nil {
		t.Fatal(err)
	}
	terraformPath := filepath.Join(binDir, "terraform")
	terraformScript := `#!/bin/sh
if [ "$1:$2" = "version:-json" ]; then
    printf '{"platform":"test_arch"}\n'
    exit 0
fi
printf ran > terraform-ran
`
	if err := os.WriteFile(terraformPath, []byte(terraformScript), 0o700); err != nil {
		t.Fatal(err)
	}
	lockFile := `provider "registry.terraform.io/delineaxpm/tss" {
  version = "5.0.0"
}
`
	if err := os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte(lockFile), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte("encrypted"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "linux", "terraform_apply.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	cmd.Env = append(environmentWithout("TFSTATE_PASSPHRASE", "TF_VAR_tss_username", "TF_VAR_tss_password", "STATE_PASSPHRASE"),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TFSTATE_PASSPHRASE=test-passphrase",
	)
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "retaining lockfile.lock") {
		t.Fatalf("wrapper result err=%v output=%s", err, output)
	}
	contents, err := os.ReadFile(filepath.Join(workDir, "terraform.tfstate"))
	if err != nil || string(contents) != "encrypted" {
		t.Fatalf("primary state contents=%q err=%v", contents, err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); err != nil {
		t.Fatalf("lock directory was not retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "terraform-ran")); !os.IsNotExist(err) {
		t.Fatalf("terraform ran after backup decryption failed: %v", err)
	}
}

func environmentWithout(keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	result := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if _, skip := blocked[key]; !skip {
			result = append(result, entry)
		}
	}
	return result
}

func TestPowerShellInitRepairsAmbiguousProviderInstall(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell is not installed")
	}

	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	currentProviderDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "5.0.0", "windows_amd64")
	staleProviderDir := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss", "4.0.0", "windows_amd64")
	for _, dir := range []string{binDir, currentProviderDir, staleProviderDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	providerScript := `#!/bin/sh
case "$1" in
state-helper-version) printf '1\n' ;;
state-helper-check-layout) ;;
decrypt) [ -e "$2" ] && printf 'decrypted' > "$2" ;;
encrypt) [ -e "$2" ] && printf 'encrypted' > "$2" ;;
*) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(currentProviderDir, "terraform-provider-tss_v5.0.0.exe"), []byte(providerScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleProviderDir, "terraform-provider-tss_v4.0.0.exe"), []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	terraformScript := `#!/bin/sh
if [ "$1:$2" = "version:-json" ]; then
    printf '{"platform":"windows_amd64"}\n'
    exit 0
fi
printf '%s\n' "$*" >> "$CALL_LOG"
if [ "$1:$2" = "init:-backend=false" ]; then
    [ ! -e terraform.tfstate ] || exit 40
    printf 'provider "registry.terraform.io/delineaxpm/tss" {\n  version = "5.0.0"\n}\n' > .terraform.lock.hcl
    exit 0
fi
[ "$(cat terraform.tfstate)" = "decrypted" ] || exit 41
`
	if err := os.WriteFile(filepath.Join(binDir, "terraform"), []byte(terraformScript), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
		if err := os.WriteFile(filepath.Join(workDir, name), []byte("encrypted"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	callLog := filepath.Join(workDir, "calls.log")
	scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "windows", "terraform_wrapper.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(workDir, "plugins with spaces")
	cmd := exec.Command(pwsh, "-NoProfile", "-File", scriptPath, "init", "--upgrade", "--plugin-dir", pluginDir)
	cmd.Dir = workDir
	cmd.Env = append(environmentWithout("TFSTATE_PASSPHRASE", "TF_VAR_tss_username", "TF_VAR_tss_password"),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TFSTATE_PASSPHRASE=test-passphrase",
		`TF_CLI_ARGS=-var="note -state=not-an-option"`,
		"CALL_LOG="+callLog,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell wrapper failed: %v\n%s", err, output)
	}
	calls, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(calls)), "init -backend=false --upgrade --plugin-dir "+pluginDir+"\ninit --upgrade --plugin-dir "+pluginDir; got != want {
		t.Fatalf("terraform calls = %q, want %q", got, want)
	}
	for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
		contents, err := os.ReadFile(filepath.Join(workDir, name))
		if err != nil || string(contents) != "encrypted" {
			t.Fatalf("%s contents=%q err=%v", name, contents, err)
		}
	}
}

func TestPowerShellInitRejectsFromModuleBeforeLocking(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell is not installed")
	}
	workDir := t.TempDir()
	scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "windows", "terraform_wrapper.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(pwsh, "-NoProfile", "-File", scriptPath, "init", "-from-module=../source-module")
	cmd.Dir = workDir
	cmd.Env = append(environmentWithout("TFSTATE_PASSPHRASE", "TF_VAR_tss_username", "TF_VAR_tss_password"), "TFSTATE_PASSPHRASE=test-passphrase")
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "does not support -from-module") {
		t.Fatalf("wrapper result = %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock directory was created: %v", err)
	}
}

func TestPowerShellWrapperRejectsUnsafeLockfileVersion(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell is not installed")
	}
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	providerRoot := filepath.Join(workDir, ".terraform", "providers", "registry.terraform.io", "delineaxpm", "tss")
	escapedRoot := filepath.Join(workDir, "escaped")
	if err := os.MkdirAll(filepath.Join(escapedRoot, "windows_amd64"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providerRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lockedVersion, err := filepath.Rel(providerRoot, escapedRoot)
	if err != nil {
		t.Fatal(err)
	}
	lockFile := "provider \"registry.terraform.io/delineaxpm/tss\" {\n  version = \"" + filepath.ToSlash(lockedVersion) + "\"\n}\n"
	if err := os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte(lockFile), 0o600); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(escapedRoot, "windows_amd64", "terraform-provider-tss_v5.0.0.exe")
	if err := os.WriteFile(providerPath, []byte("#!/bin/sh\nprintf invoked > \"$MARKER\"\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	terraformPath := filepath.Join(binDir, "terraform")
	if err := os.WriteFile(terraformPath, []byte("#!/bin/sh\n[ \"$1:$2\" = \"version:-json\" ] && printf '{\"platform\":\"windows_amd64\"}\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "windows", "terraform_wrapper.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(workDir, "provider-invoked")
	cmd := exec.Command(pwsh, "-NoProfile", "-File", scriptPath, "apply")
	cmd.Dir = workDir
	cmd.Env = append(environmentWithout("TFSTATE_PASSPHRASE", "TF_VAR_tss_username", "TF_VAR_tss_password"),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TFSTATE_PASSPHRASE=test-passphrase",
		"MARKER="+marker,
	)
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "provider version in .terraform.lock.hcl is invalid") {
		t.Fatalf("wrapper result err=%v output=%s", err, output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("escaped provider was invoked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock directory was created: %v", err)
	}
}

func TestPowerShellWrappersRejectStateRedirectionBeforeLocking(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("PowerShell is not installed")
	}
	tests := []struct {
		operation string
		argument  string
		env       string
		message   string
	}{
		{operation: "init", argument: "-backend-config=backend.hcl", message: "does not support -backend-config"},
		{operation: "init", argument: "--backend-config=backend.hcl", message: "does not support -backend-config"},
		{operation: "init", argument: "--from-module=../source-module", message: "does not support -from-module"},
		{operation: "apply", argument: "-state=alternate.tfstate", message: "does not support -state"},
		{operation: "apply", argument: "--state=alternate.tfstate", message: "does not support -state"},
		{operation: "destroy", argument: "-backup", message: "does not support -state"},
		{operation: "destroy", argument: "--backup", message: "does not support -state"},
		{operation: "apply", env: "TF_CLI_ARGS_apply=--state-out=alternate.tfstate", message: "redirects state"},
		{operation: "apply", env: `TF_CLI_ARGS_apply=--sta"te"=alternate.tfstate`, message: "redirects state"},
		{operation: "destroy", env: `TF_CLI_ARGS_destroy=-sta\te-out=alternate.tfstate`, message: "redirects state"},
		{operation: "init", env: "TF_CLI_ARGS_init=--backend-config=backend.hcl", message: "does not support"},
		{operation: "init", env: `TF_CLI_ARGS_init=--backend-"config"=backend.hcl`, message: "does not support"},
		{operation: "init", env: "TF_CLI_ARGS_init=--from-module=../source-module", message: "does not support"},
	}
	for _, test := range tests {
		t.Run(test.operation+"/"+test.argument+test.env, func(t *testing.T) {
			workDir := t.TempDir()
			scriptPath, err := filepath.Abs(filepath.Join("..", "encryption_scripts", "windows", "terraform_wrapper.ps1"))
			if err != nil {
				t.Fatal(err)
			}
			arguments := []string{"-NoProfile", "-File", scriptPath, test.operation}
			if test.argument != "" {
				arguments = append(arguments, test.argument)
			}
			cmd := exec.Command(pwsh, arguments...)
			cmd.Dir = workDir
			cmd.Env = append(environmentWithout("TFSTATE_PASSPHRASE", "TF_CLI_ARGS", "TF_CLI_ARGS_init", "TF_CLI_ARGS_apply", "TF_CLI_ARGS_destroy"), "TFSTATE_PASSPHRASE=test-passphrase")
			if test.env != "" {
				cmd.Env = append(cmd.Env, test.env)
			}
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.message) {
				t.Fatalf("wrapper result = %v\n%s", err, output)
			}
			if _, err := os.Stat(filepath.Join(workDir, "lockfile.lock")); !os.IsNotExist(err) {
				t.Fatalf("lock directory was created: %v", err)
			}
		})
	}
}
