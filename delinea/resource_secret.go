package delinea

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceSecret() *schema.Resource {
	return &schema.Resource{
		Create: dataSourceSecretCreate,
		Read:   dataSourceSecretReadNew,
		Update: dataSourceSecretUpdate,
		Delete: dataSourceSecretDelete,
		Schema: getSecretSchema(),
	}
}

func dataSourceSecretReadNew(d *schema.ResourceData, meta interface{}) error {
	id, err := strconv.Atoi(d.Id())
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

	if secret == nil {
		return fmt.Errorf("the secret does not present")
	}

	d.SetId(strconv.Itoa(secret.ID))

	d.Set("active", secret.Active)
	d.Set("folderid", secret.FolderID)
	d.Set("name", secret.Name)
	d.Set("secrettemplateid", secret.SecretTemplateID)
	d.Set("siteid", secret.SiteID)
	d.Set("autochangenabled", secret.AutoChangeEnabled)
	d.Set("checkedout", secret.CheckedOut)
	d.Set("checkoutchangepasswordenabled", secret.CheckOutChangePasswordEnabled)
	d.Set("checkoutenabled", secret.CheckOutEnabled)
	d.Set("delayindexing", secret.DelayIndexing)
	d.Set("enableinheritpermissions", secret.EnableInheritPermissions)
	d.Set("enableinheritsecretpolicy", secret.EnableInheritSecretPolicy)
	d.Set("launcherconnectassecretid", secret.LauncherConnectAsSecretID)
	d.Set("passwordtypewebscriptid", secret.PasswordTypeWebScriptID)
	d.Set("proxyenabled", secret.ProxyEnabled)
	d.Set("requirescomment", secret.RequiresComment)
	d.Set("secretpolicyid", secret.SecretPolicyID)
	d.Set("sessionrecordingenabled", secret.SessionRecordingEnabled)
	d.Set("weblauncherrequiresincognitomode", secret.WebLauncherRequiresIncognitoMode)

	finalFields := make([]map[string]interface{}, 0)

	for _, secretField := range secret.Fields {
		fields := d.Get("fields").([]interface{})
		fieldName := secretField.FieldName

		// Check if field exists in our state, if it doesn't don't add it
		// to the list because terraform will try to "remove" it
		if fields != nil && len(fields) != 0 {
			found := fieldExistsInMap(fields, fieldName)
			if !found {
				log.Print("[DEBUG] Not found field: ", fieldName)
				continue
			}
		}
		// If value returned from the API is empty and the field doesn't exist
		// in our state or the list is empty, skip adding the field.
		// This can happen when importing a secret to terraform.
		if secretField.ItemValue == "" && !fieldExistsInMap(fields, fieldName) {
			continue
		}

		field := make(map[string]interface{})

		field["fieldname"] = secretField.FieldName
		field["fielddescription"] = secretField.FieldDescription
		field["fieldid"] = secretField.FieldID
		field["fileattachmentid"] = secretField.FileAttachmentID
		field["isfile"] = secretField.IsFile
		field["isnotes"] = secretField.IsNotes
		field["ispassword"] = secretField.IsPassword
		field["itemvalue"] = secretField.ItemValue
		field["slug"] = secretField.Slug

		finalFields = append(finalFields, field)
	}
	d.Set("fields", finalFields)

	return nil
}

func dataSourceSecretDelete(d *schema.ResourceData, meta interface{}) error {

	id, err := strconv.Atoi(d.Id())

	secrets, err := server.New(meta.(server.Configuration))
	if err != nil {
		log.Printf("[DEBUG] configuration error: %s", err)
	}

	log.Printf("[DEBUG] deleting secret with id %d", id)

	err = secrets.DeleteSecret(id)
	if err != nil {
		return err
	}

	log.Printf("Secret is Deleted successfully...!")

	return nil
}

func dataSourceSecretUpdate(d *schema.ResourceData, meta interface{}) error {
	id, err := strconv.Atoi(d.Id())
	secrets, err := server.New(meta.(server.Configuration))
	if err != nil {
		log.Printf("[DEBUG] configuration error: %s", err)
	}

	log.Printf("[DEBUG] updating secret with id %d", id)

	refSecret := new(server.Secret)
	refSecret.ID = id
	err = getSecretData(d, refSecret, secrets)
	if err != nil {
		return err
	}

	sc, err := secrets.UpdateSecret(*refSecret)
	if err != nil {
		log.Printf("calling server.UpdateSecret: %s", err)
		return err
	}
	if sc == nil {
		log.Printf("updated secret data is nil")
		return nil
	}

	log.Printf("Secret is Updated successfully...!")

	d.SetId(strconv.Itoa(sc.ID))

	return dataSourceSecretReadNew(d, meta)
}

func dataSourceSecretCreate(d *schema.ResourceData, meta interface{}) error {
	secrets, err := server.New(meta.(server.Configuration))
	if err != nil {
		log.Printf("[DEBUG] configuration error: %s", err)
	}

	refSecret := new(server.Secret)

	err = getSecretData(d, refSecret, secrets)
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] creating secret with name %s", refSecret.Name)

	sc, err := secrets.CreateSecret(*refSecret)
	if err != nil {
		log.Printf("calling server.CreateSecret: %s", err)
		return err
	}
	if sc == nil {
		log.Printf("created secret data is nil")
		return nil
	}

	log.Printf("Secret is Created successfully...!")

	d.SetId(strconv.Itoa(sc.ID))

	return dataSourceSecretReadNew(d, meta)
}

func getSecretData(d *schema.ResourceData, object *server.Secret, secrets *server.Server) error {
	object.Name = d.Get("name").(string)
	object.SiteID = d.Get("siteid").(int)
	object.FolderID = d.Get("folderid").(int)
	object.SecretTemplateID = d.Get("secrettemplateid").(int)

	template, err := secrets.SecretTemplate(object.SecretTemplateID)

	if err != nil {
		log.Print("[DEBUG] unable to get secret template", err)
		return err
	}

	if d.Get("fields") != nil {
		fields := d.Get("fields").([]interface{})

		for _, p := range fields {
			field := server.SecretField{}
			templateField := server.SecretTemplateField{}
			fieldName := ""
			if value, ok := p.(map[string]interface{})["fieldname"]; ok && value != nil {
				fieldName = value.(string)
			}

			for _, record := range template.Fields {
				if strings.ToLower(record.Name) == strings.ToLower(fieldName) || strings.ToLower(record.FieldSlugName) == strings.ToLower(fieldName) {
					templateField = record
				}
			}

			field.FieldDescription = templateField.Description
			field.FieldID = templateField.SecretTemplateFieldID
			field.FieldName = templateField.Name
			if value, ok := p.(map[string]interface{})["fileattachmentid"]; ok && value != nil {
				field.FileAttachmentID = value.(int)
			}
			if value, ok := p.(map[string]interface{})["filename"]; ok && value != nil {
				field.Filename = value.(string)
			}
			field.IsFile = templateField.IsFile
			//field.IsList = templateField.IsList
			field.IsNotes = templateField.IsNotes
			field.IsPassword = templateField.IsPassword
			if value, ok := p.(map[string]interface{})["itemvalue"]; ok && value != nil {
				field.ItemValue = value.(string)
			}
			//field.ListType = templateField.ListType
			field.Slug = templateField.FieldSlugName

			object.Fields = append(object.Fields, field)
		}
	}

	if value := d.Get("secretpolicyid"); value != nil {
		object.SecretPolicyID = value.(int)
	}
	if value := d.Get("passwordtypewebscriptid"); value != nil {
		object.PasswordTypeWebScriptID = value.(int)
	}
	if value := d.Get("launcherconnectassecretid"); value != nil {
		object.LauncherConnectAsSecretID = value.(int)
	}
	if value := d.Get("checkoutintervalminutes"); value != nil {
		object.CheckOutIntervalMinutes = value.(int)
	}
	if value := d.Get("active"); value != nil {
		object.Active = value.(bool)
	} else {
		object.Active = true
	}
	if value := d.Get("checkedout"); value != nil {
		object.CheckedOut = value.(bool)
	}
	if value := d.Get("checkoutenabled"); value != nil {
		object.CheckOutEnabled = value.(bool)
	}
	if value := d.Get("autochangenabled"); value != nil {
		object.AutoChangeEnabled = value.(bool)
	}
	if value := d.Get("checkoutchangepasswordenabled"); value != nil {
		object.CheckOutChangePasswordEnabled = value.(bool)
	}
	if value := d.Get("delayindexing"); value != nil {
		object.DelayIndexing = value.(bool)
	}
	if value := d.Get("enableinheritpermissions"); value != nil {
		object.EnableInheritPermissions = value.(bool)
	}
	if value := d.Get("enableinheritsecretpolicy"); value != nil {
		object.EnableInheritSecretPolicy = value.(bool)
	}
	if value := d.Get("proxyenabled"); value != nil {
		object.ProxyEnabled = value.(bool)
	}
	if value := d.Get("requirescomment"); value != nil {
		object.RequiresComment = value.(bool)
	}
	if value := d.Get("sessionrecordingenabled"); value != nil {
		object.SessionRecordingEnabled = value.(bool)
	}
	if value := d.Get("weblauncherrequiresincognitomode"); value != nil {
		object.WebLauncherRequiresIncognitoMode = value.(bool)
	}

	return nil
}

func fieldExistsInMap(fields []interface{}, fieldName string) bool {
	for _, field := range fields {
		if field.(map[string]interface{})["fieldname"] == fieldName {
			log.Print("[DEBUG] Found field: ", fieldName)
			return true
		}
	}
	return false
}

func getSecretSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"name": {
			Description: "the name of the secret",
			Required:    true,
			Type:        schema.TypeString,
		},
		"folderid": {
			Description: "the foleder id of the secret",
			Required:    true,
			Type:        schema.TypeInt,
		},
		"siteid": {
			Description: "the id of the site where secret will create",
			Required:    true,
			Type:        schema.TypeInt,
		},
		"secrettemplateid": {
			Description: "the id of the template in which secret will create",
			Required:    true,
			Type:        schema.TypeInt,
			ForceNew:    true,
		},
		"secretpolicyid": {
			Description: "the id of the secret policy",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeInt,
		},
		"passwordtypewebscriptid": {
			Description: "the id of the password type webscript",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeInt,
		},
		"launcherconnectassecretid": {
			Description: "the id of the launcher connect as secret",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeInt,
		},
		"checkoutintervalminutes": {
			Description: "the secret checkout interval minutes",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeInt,
		},
		"active": {
			Description: "the secret is enabled or disabled",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"checkedout": {
			Description: "the secret is checked out or not",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"checkoutenabled": {
			Description: "the secret checkout enabled or disabled",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"autochangenabled": {
			Description: "the autochange is enabled or disabled",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"checkoutchangepasswordenabled": {
			Description: "the checkout change password enabled or disabled",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"delayindexing": {
			Description: "the delay indexing is enabled or disabled",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"enableinheritpermissions": {
			Description: "the inherit permission is enabled or disabled",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"enableinheritsecretpolicy": {
			Description: "the inherit secret policy is enabled or disabled",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"proxyenabled": {
			Description: "the proxy enabled or disabled",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"requirescomment": {
			Description: "the comment is required or not",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"sessionrecordingenabled": {
			Description: "the session recording is enabled or disabled",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"weblauncherrequiresincognitomode": {
			Description: "the secret requires web launcher encognito mode or not",
			Optional:    true,
			Computed:    true,
			Type:        schema.TypeBool,
		},
		"fields": {
			Description: "the fields of the secret",
			Required:    true,
			Type:        schema.TypeList,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"fieldid": {
						Type:     schema.TypeInt,
						Optional: true,
						Computed: true,
					},
					"fileattachmentid": {
						Type:     schema.TypeInt,
						Optional: true,
						Computed: true,
					},
					"fieldname": {
						Type:     schema.TypeString,
						Required: true,
					},
					"slug": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"fielddescription": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"filename": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"itemvalue": {
						Type:     schema.TypeString,
						Optional: true,
						Computed: true,
					},
					"isfile": {
						Type:     schema.TypeBool,
						Optional: true,
						Computed: true,
					},
					"isnotes": {
						Type:     schema.TypeBool,
						Optional: true,
						Computed: true,
					},
					"ispassword": {
						Type:     schema.TypeBool,
						Optional: true,
						Computed: true,
					},
					"islist": {
						Type:     schema.TypeBool,
						Optional: true,
					},
					"listtype": {
						Type:     schema.TypeString,
						Optional: true,
					},
				},
			},
		},
		"sshkeyargs": {
			Description: "the ssh key arguments of the secret",
			Optional:    true,
			Type:        schema.TypeSet,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"generatepassphrase": {
						Type:     schema.TypeBool,
						Required: true,
					},
					"generatesshkey": {
						Type:     schema.TypeBool,
						Required: true,
					},
				},
			},
		},
	}
}
