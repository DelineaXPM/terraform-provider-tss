package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/DelineaXPM/terraform-provider-tss/v5/delinea"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version and commit are set by goreleaser via -ldflags (see .goreleaser.yml).
var (
	version = "dev"
	commit  = ""
)

const usage = `usage:
  terraform-provider-tss [-debug]                 serve the provider (invoked by Terraform)
  terraform-provider-tss -version                 print the build version
  terraform-provider-tss state-helper-check-layout
                                                  verify the wrapper-supported state layout
  terraform-provider-tss encrypt|decrypt <file>   encrypt or decrypt a local state file with
                                                  the passphrase in TFSTATE_PASSPHRASE
`

const stateHelperProtocolVersion = "1"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 1 && args[0] == "state-helper-version" {
		fmt.Println(stateHelperProtocolVersion)
		return 0
	}
	if len(args) == 1 && args[0] == "state-helper-check-layout" {
		if err := delinea.ValidateEncryptedStateLayout(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if len(args) > 0 && (args[0] == "encrypt" || args[0] == "decrypt") {
		return runStateFileCommand(args)
	}

	fs := flag.NewFlagSet("terraform-provider-tss", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	debug := fs.Bool("debug", false, "run the provider in debug mode for attaching a debugger")
	showVersion := fs.Bool("version", false, "print the build version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	if *showVersion {
		if commit != "" {
			fmt.Printf("terraform-provider-tss %s (%s)\n", version, commit)
		} else {
			fmt.Printf("terraform-provider-tss %s\n", version)
		}
		return 0
	}

	err := providerserver.Serve(context.Background(), delinea.New, providerserver.ServeOpts{
		Address: "registry.terraform.io/delineaxpm/tss",
		Debug:   *debug,
	})
	if err != nil {
		log.Printf("provider server exited with error: %v", err)
		return 1
	}
	return 0
}

// runStateFileCommand implements the encrypt/decrypt helper used by the
// wrapper scripts in encryption_scripts/. It exits non-zero on any failure so
// the scripts' error checks are meaningful.
func runStateFileCommand(args []string) int {
	if len(args) != 2 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	action, stateFile := args[0], args[1]

	passphrase := os.Getenv("TFSTATE_PASSPHRASE")
	if passphrase == "" {
		fmt.Fprintln(os.Stderr, "TFSTATE_PASSPHRASE environment variable is not set")
		return 2
	}

	var err error
	switch action {
	case "encrypt":
		err = delinea.EncryptFile(passphrase, stateFile)
	case "decrypt":
		err = delinea.DecryptFile(passphrase, stateFile)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s: %v\n", action, stateFile, err)
		return 1
	}
	return 0
}
