package delinea

import (
	"fmt"
	"log"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceSecretsRead(d *schema.ResourceData, meta interface{}) error {
	field := d.Get("field").(string)
	secrets, err := server.New(meta.(server.Configuration))

	if err != nil {
		log.Printf("[DEBUG] configuration error: %s", err)
		return err
	}

	ids := d.Get("ids").([]interface{})
	if len(ids) == 0 {
		return fmt.Errorf("no secret IDs provided")
	}

	var results []map[string]interface{}

	for _, id := range ids {
		secretID, ok := id.(int)
		if !ok {
			return fmt.Errorf("invalid ID format: %v", id)
		}

		log.Printf("[DEBUG] getting secret with id %d", secretID)

		secret, err := secrets.Secret(secretID)
		if err != nil {
			log.Printf("[DEBUG] unable to get secret with id %d: %s", secretID, err)
			continue // Skip this ID and continue with the rest
		}

		secretData := map[string]interface{}{
			"id": secretID,
		}

		if value, ok := secret.Field(field); ok {
			secretData["value"] = value
		} else {
			secretData["value"] = fmt.Sprintf("field '%s' not found", field)
		}

		results = append(results, secretData)
	}

	if err := d.Set("secrets", results); err != nil {
		return err
	}

	d.SetId("multiple_secrets")

	return nil
}

func dataSourceSecrets() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceSecretsRead,

		Schema: map[string]*schema.Schema{
			"secrets": {
				Computed:    true,
				Description: "a list of secrets with their field values",
				Type:        schema.TypeList,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Description: "the id of the secret",
							Type:        schema.TypeInt,
							Required:    true,
						},
						"value": {
							Description: "the value of the field of the secret",
							Type:        schema.TypeString,
							Computed:    true,
							Sensitive:   true,
						},
					},
				},
			},
			"field": {
				Description: "the field to extract from the secret",
				Required:    true,
				Type:        schema.TypeString,
			},
			"ids": {
				Description: "a list of IDs of the secrets",
				Required:    true,
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeInt},
			},
		},
	}
}
