package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

// type Secret struct {
// 	Name                                                                       string
// 	FolderID, ID, SiteID, SecretTemplateID                                     int
// 	SecretPolicyID, PasswordTypeWebScriptID                                    int `json:",omitempty"`
// 	LauncherConnectAsSecretID, CheckOutIntervalMinutes                         int
// 	Active, CheckedOut, CheckOutEnabled                                        bool
// 	AutoChangeEnabled, CheckOutChangePasswordEnabled, DelayIndexing            bool
// 	EnableInheritPermissions, EnableInheritSecretPolicy, ProxyEnabled          bool
// 	RequiresComment, SessionRecordingEnabled, WebLauncherRequiresIncognitoMode bool
// 	Fields                                                                     []SecretField `json:"Items"`
// 	SshKeyArgs                                                                 *SshKeyArgs   `json:",omitempty"`
// }

// type SecretField struct {
// 	FieldID   int
// 	ItemValue string
// }

// type SshKeyArgs struct {
// 	GeneratePassphrase, GenerateSshKeys bool
// }

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

	d.SetId(strconv.Itoa(secret.ID))

	if secret != nil {
		return nil
	}

	return fmt.Errorf("the secret does not present")
}

func dataSourceSecretDelete(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func dataSourceSecretUpdate(d *schema.ResourceData, meta interface{}) error {
	// folderId := d.Get("folder_id").(int)
	// siteId := d.Get("siteid").(int)
	// secretTemplateId := d.Get("secret_template_id").(int)
	// name := d.Get("name").(string)
	// secrets, err := server.New(meta.(server.Configuration))

	// fields := []server.SecretField{}
	// if d.Get("fields") != nil {
	// 	for _, p := range d.Get("fields").([]interface{}) {
	// 		field := server.SecretField{}
	// 		field.FieldID = p.(map[string]interface{})["field_id"].(int)
	// 		field.ItemValue = p.(map[string]interface{})["item_value"].(string)
	// 		fields = append(fields, field)
	// 	}
	// }

	// if err != nil {
	// 	log.Printf("[DEBUG] configuration error: %s", err)
	// }
	// log.Printf("[DEBUG] updating secret with name %s", name)
	return nil
}

func dataSourceSecretCreate(d *schema.ResourceData, meta interface{}) error {
	folderId := d.Get("folder_id").(int)
	siteId := d.Get("siteid").(int)
	secretTemplateId := d.Get("secret_template_id").(int)
	name := d.Get("name").(string)
	//fields := d.Get("fields").([]map[string]interface{})
	secrets, err := server.New(meta.(server.Configuration))

	fields := []server.SecretField{}
	if d.Get("fields") != nil {
		for _, p := range d.Get("fields").([]interface{}) {
			field := server.SecretField{}
			field.FieldID = p.(map[string]interface{})["field_id"].(int)
			field.ItemValue = p.(map[string]interface{})["item_value"].(string)
			fields = append(fields, field)
		}
	}

	if err != nil {
		log.Printf("[DEBUG] configuration error: %s", err)
	}
	log.Printf("[DEBUG] creating secret with name %s", name)

	refSecret := new(server.Secret)
	refSecret.Name = name
	refSecret.SiteID = siteId
	refSecret.FolderID = folderId
	refSecret.SecretTemplateID = secretTemplateId
	refSecret.Fields = make([]server.SecretField, len(fields))
	refSecret.Fields[0].FieldID = fields[0].FieldID
	refSecret.Fields[0].ItemValue = fields[0].ItemValue
	refSecret.Fields[1].FieldID = fields[1].FieldID
	refSecret.Fields[1].ItemValue = fields[1].ItemValue
	refSecret.Fields[2].FieldID = fields[2].FieldID
	refSecret.Fields[2].ItemValue = fields[2].ItemValue
	refSecret.Fields[3].FieldID = fields[3].FieldID
	refSecret.Fields[3].ItemValue = fields[3].ItemValue
	sc, err := secrets.CreateSecret(*refSecret)
	if err != nil {
		log.Printf("calling server.CreateSecret: %s", err)
		return err
	}
	if sc == nil {
		log.Printf("created secret data is nil")
		return nil
	}

	if err != nil {
		log.Printf("[DEBUG] unable to get secret: %s", err)
		return err
	}

	log.Printf("Secret is Created successfully...!")

	d.SetId(strconv.Itoa(sc.ID))

	return dataSourceSecretReadNew(d, meta)
}

func getSecretSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"name": {
			Description: "the name of the secret",
			Required:    true,
			Type:        schema.TypeString,
			ForceNew:    true,
		},
		"folder_id": {
			Description: "the foleder id of the secret",
			Required:    true,
			Type:        schema.TypeInt,
			ForceNew:    true,
		},
		"siteid": {
			Description: "the id of the site where secret will create",
			Required:    true,
			Type:        schema.TypeInt,
			ForceNew:    true,
		},
		"secret_template_id": {
			Description: "the id of the template in which secret will create",
			Required:    true,
			Type:        schema.TypeInt,
			ForceNew:    true,
		},
		"secret_policy_id": {
			Description: "the id of the secret policy",
			Optional:    true,
			Type:        schema.TypeInt,
			ForceNew:    true,
		},
		"password_type_web_script_id": {
			Description: "the id of the password type webscript",
			Optional:    true,
			Type:        schema.TypeInt,
			ForceNew:    true,
		},
		"launcher_connect_as_secretid": {
			Description: "the id of the launcher connect as secret",
			Optional:    true,
			Type:        schema.TypeInt,
			ForceNew:    true,
		},
		"checkout_interval_minutes": {
			Description: "the secret checkout interval minutes",
			Optional:    true,
			Type:        schema.TypeInt,
			ForceNew:    true,
		},
		"active": {
			Description: "the secret is enabled or disabled",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"checkedout": {
			Description: "the secret is checked out or not",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"checkout_enabled": {
			Description: "the secret checkout enabled or disabled",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"auto_change_enabled": {
			Description: "the autochange is enabled or disabled",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"checkout_change_password_enabled": {
			Description: "the checkout change password enabled or disabled",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"delay_indexing": {
			Description: "the delay indexing is enabled or disabled",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"enable_inherit_permissions": {
			Description: "the inherit permission is enabled or disabled",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"enable_inherit_secret_policy": {
			Description: "the inherit secret policy is enabled or disabled",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"proxy_enabled": {
			Description: "the proxy enabled or disabled",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"requires_comment": {
			Description: "the comment is required or not",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"session_recording_enabled": {
			Description: "the session recording is enabled or disabled",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"web_launcher_requires_incognito_mode": {
			Description: "the secret requires web launcher encognito mode or not",
			Optional:    true,
			Type:        schema.TypeBool,
			ForceNew:    true,
		},
		"fields": {
			Description: "the fields of the secret",
			Required:    true,
			Type:        schema.TypeList,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"field_id": {
						Type:     schema.TypeInt,
						Required: true,
					},
					"item_value": {
						Type:     schema.TypeString,
						Required: true,
					},
				},
			},
			ForceNew: true,
		},
		"ssh_key_args": {
			Description: "the ssh key arguments of the secret",
			Optional:    true,
			Type:        schema.TypeSet,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"generate_passphrase": {
						Type:     schema.TypeBool,
						Required: true,
					},
					"generate_ssh_key": {
						Type:     schema.TypeBool,
						Required: true,
					},
				},
			},
			ForceNew: true,
		},
	}
}
