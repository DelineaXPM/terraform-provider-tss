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
			"secrettemplatefieldid": field.SecretTemplateFieldID,
			"fieldslugname":         field.FieldSlugName,
			"displayname":           field.DisplayName,
			"description":           field.Description,
			"name":                  field.Name,
			"listtype":              field.ListType,
			"isfile":                field.IsFile,
			"islist":                field.IsList,
			"isnotes":               field.IsNotes,
			"ispassword":            field.IsPassword,
			"isrequired":            field.IsRequired,
			"isurl":                 field.IsUrl,
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
						"secrettemplatefieldid": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"fieldslugname": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"displayname": {
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
						"listtype": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"isfile": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"islist": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"isnotes": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"ispassword": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"isrequired": {
							Type:     schema.TypeBool,
							Computed: true,
						},
						"isurl": {
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
