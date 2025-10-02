package main

import (
	"context"
	"log"
	"os"

	"github.com/DelineaXPM/terraform-provider-tss/v3/delinea"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

func main() {
	if len(os.Args) >= 2 {
		action := os.Args[1]
		stateFile := os.Args[2]

		passphrase := os.Getenv("TFSTATE_PASSPHRASE")
		if passphrase == "" {
			log.Println("Passphrase not set in TFSTATE_PASSPHRASE environment variable")
			return
		}

		switch action {
		case "encrypt":
			err := delinea.EncryptFile(passphrase, stateFile)
			if err != nil {
				log.Printf("[DEBUG] Error encrypting file: %v\n", err)
			}
		case "decrypt":
			err := delinea.DecryptFile(passphrase, stateFile)
			if err != nil {
				log.Printf("[DEBUG] Error decrypting file: %v\n", err)
			}
		default:
			log.Println("[DEBUG] Invalid action. Use 'encrypt' or 'decrypt'.")
		}
		return
	}

	providerserver.Serve(context.Background(), delinea.New, providerserver.ServeOpts{
		Address: "registry.terraform.io/DelineaXPM/tss",
	})
}
