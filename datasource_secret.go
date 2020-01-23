package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/thycotic/tss-sdk-go/server"
)

func dataSourceSecretRead(d *schema.ResourceData, meta interface{}) error {
	id := d.Get("id").(int)
	field := d.Get("field").(string)
	secrets, err := server.New(meta.(server.Configuration))

	if err != nil {
		log.Printf("[DEBUG] configuration error: %s", err)
	}
	log.Printf("[DEBUG] getting secret with id %d", id)

	secret, err := secrets.Secret(id)

	if err != nil {
		log.Print("[DEBUG] unable to get secret", err)
		return err
	}

	d.SetId(strconv.Itoa(secret.ID))

	log.Printf("[DEBUG] using '%s' field of secret with id %d", field, id)

	if value, ok := secret.Field(field); ok {
		d.Set("value", value)
		return nil
	}
	return fmt.Errorf("the secret does not contain a '%s' field", field)
}

func dataSourceSecret() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceSecretRead,

		Schema: map[string]*schema.Schema{
			"value": {
				Computed:    true,
				Description: "the value of the field of the secret",
				Sensitive:   true,
				Type:        schema.TypeString,
			},
			"field": {
				Description: "the field to extract from the secret",
				Required:    true,
				Type:        schema.TypeString,
			},
			"id": {
				Description: "the id of the secret",
				Required:    true,
				Type:        schema.TypeInt,
			},
		},
	}
}
