package delinea

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

const unsupportedStateLayout = "the encrypted-state wrappers support only the default workspace and the local backend's default terraform.tfstate paths"

type backendMetadata struct {
	Backend *struct {
		Type   string                     `json:"type"`
		Config map[string]json.RawMessage `json:"config"`
	} `json:"backend"`
}

func ValidateEncryptedStateLayout() error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determine Terraform working directory: %w", err)
	}
	dataDir := os.Getenv("TF_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(workDir, ".terraform")
	} else if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(workDir, dataDir)
	}
	return validateEncryptedStateLayout(workDir, dataDir, os.Getenv("TF_WORKSPACE"))
}

func validateEncryptedStateLayout(workDir, dataDir, environmentWorkspace string) error {
	if err := validateConfiguredBackend(workDir); err != nil {
		return err
	}
	workspace := environmentWorkspace
	if workspace == "" {
		contents, err := os.ReadFile(filepath.Join(dataDir, "environment"))
		switch {
		case err == nil:
			workspace = strings.TrimSpace(string(contents))
		case !os.IsNotExist(err):
			return fmt.Errorf("read Terraform workspace selection: %w", err)
		}
	}
	if workspace != "" && workspace != "default" {
		return fmt.Errorf("%s; selected workspace is %q", unsupportedStateLayout, workspace)
	}

	contents, err := os.ReadFile(filepath.Join(dataDir, "terraform.tfstate"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read initialized Terraform backend metadata: %w", err)
	}
	var metadata backendMetadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return fmt.Errorf("parse initialized Terraform backend metadata: %w", err)
	}
	if metadata.Backend == nil {
		return nil
	}
	if metadata.Backend.Type != "local" {
		return fmt.Errorf("%s; initialized backend is %q", unsupportedStateLayout, metadata.Backend.Type)
	}
	if err := validateMetadataPath(workDir, metadata.Backend.Config, "path", "terraform.tfstate"); err != nil {
		return err
	}
	return validateMetadataPath(workDir, metadata.Backend.Config, "workspace_dir", "terraform.tfstate.d")
}

func validateConfiguredBackend(workDir string) error {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return fmt.Errorf("read Terraform working directory: %w", err)
	}
	parser := hclparse.NewParser()
	backendCount := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (!strings.HasSuffix(name, ".tf") && !strings.HasSuffix(name, ".tf.json")) {
			continue
		}
		path := filepath.Join(workDir, name)
		var file *hcl.File
		var diagnostics hcl.Diagnostics
		if strings.HasSuffix(name, ".json") {
			file, diagnostics = parser.ParseJSONFile(path)
		} else {
			file, diagnostics = parser.ParseHCLFile(path)
		}
		if diagnostics.HasErrors() {
			return fmt.Errorf("parse Terraform configuration %s before decrypting state: %s", name, diagnostics.Error())
		}
		terraformContent, _, _ := file.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}}})
		for _, terraformBlock := range terraformContent.Blocks {
			backendContent, _, _ := terraformBlock.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
				{Type: "backend", LabelNames: []string{"type"}},
				{Type: "cloud"},
			}})
			for _, backendBlock := range backendContent.Blocks {
				if backendBlock.Type == "cloud" {
					return fmt.Errorf("%s; the root module configures Terraform cloud state", unsupportedStateLayout)
				}
				backendCount++
				if len(backendBlock.Labels) != 1 || backendBlock.Labels[0] != "local" {
					backendType := "unknown"
					if len(backendBlock.Labels) == 1 {
						backendType = backendBlock.Labels[0]
					}
					return fmt.Errorf("%s; configured backend is %q", unsupportedStateLayout, backendType)
				}
				attributes, attributeDiagnostics := backendBlock.Body.JustAttributes()
				if attributeDiagnostics.HasErrors() {
					return fmt.Errorf("parse local backend configuration before decrypting state: %s", attributeDiagnostics.Error())
				}
				if err := validateConfiguredPath(workDir, attributes, "path", "terraform.tfstate"); err != nil {
					return err
				}
				if err := validateConfiguredPath(workDir, attributes, "workspace_dir", "terraform.tfstate.d"); err != nil {
					return err
				}
			}
		}
	}
	if backendCount > 1 {
		return fmt.Errorf("multiple backend blocks were found; refusing to decrypt state until the Terraform configuration is valid")
	}
	return nil
}

func validateConfiguredPath(workDir string, attributes hcl.Attributes, name, expected string) error {
	attribute, ok := attributes[name]
	if !ok {
		return nil
	}
	value, diagnostics := attribute.Expr.Value(nil)
	if diagnostics.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String {
		return fmt.Errorf("%s; local backend %s must be the literal default %q", unsupportedStateLayout, name, expected)
	}
	if !pathsEquivalent(workDir, value.AsString(), expected) {
		return fmt.Errorf("%s; local backend %s is %q", unsupportedStateLayout, name, value.AsString())
	}
	return nil
}

func validateMetadataPath(workDir string, config map[string]json.RawMessage, name, expected string) error {
	raw, ok := config[name]
	if !ok || string(raw) == "null" {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("parse initialized local backend %s: %w", name, err)
	}
	if !pathsEquivalent(workDir, value, expected) {
		return fmt.Errorf("%s; initialized local backend %s is %q", unsupportedStateLayout, name, value)
	}
	return nil
}

func pathsEquivalent(workDir, actual, expected string) bool {
	if actual == "" {
		return false
	}
	actualPath := actual
	if !filepath.IsAbs(actualPath) {
		actualPath = filepath.Join(workDir, actualPath)
	}
	expectedPath := filepath.Join(workDir, expected)
	return filepath.Clean(actualPath) == filepath.Clean(expectedPath)
}
