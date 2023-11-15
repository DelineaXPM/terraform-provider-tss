package main

import (
	"log"
	"strconv"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func dataSourceSecretTemplateRead(d *schema.ResourceData, meta interface{}) error {
	id := d.Get("id").(int)
	secrets, err := server.New(meta.(server.Configuration))

	if err != nil {
		log.Printf("[DEBUG] configuration error: %s", err)
	}
	log.Printf("[DEBUG] getting secret template with id %d", id)

	template, err := secrets.SecretTemplate(id)

	if err != nil {
		log.Print("[DEBUG] unable to get secret template", err)
		return err
	}

	d.SetId(strconv.Itoa(template.ID))
	d.Set("name", template.Name)

	var flattenedFields []map[string]interface{}
	for _, field := range template.Fields {
		flattenedFields = append(flattenedFields, map[string]interface{}{
			"secret_template_field_id": field.SecretTemplateFieldID,
			"field_slug_name":          field.FieldSlugName,
			"display_name":             field.DisplayName,
			"description":              field.Description,
			"name":                     field.Name,
			"list_type":                field.ListType,
			"is_file":                  field.IsFile,
			"is_list":                  field.IsList,
			"is_notes":                 field.IsNotes,
			"is_password":              field.IsPassword,
			"is_required":              field.IsRequired,
			"is_url":                   field.IsUrl,
		})
	}

	d.Set("fields", flattenedFields)
	return nil
}

func dataSourceSecretTemplate() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceSecretTemplateRead,

		Schema: map[string]*schema.Schema{
			"fields": {
				Description: "the fields of the secret template",
				Computed:    true,
				Type:        schema.TypeList,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"secret_template_field_id": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"field_slug_name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"display_name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"description": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"list_type": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"is_file": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"is_list": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"is_notes": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"is_password": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"is_required": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"is_url": {
							Type:     schema.TypeBool,
							Computed: true,
						},
					},
				},
			},
			"name": {
				Computed:    true,
				Description: "the name of the secret template",
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
