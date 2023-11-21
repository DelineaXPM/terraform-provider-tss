package main

import (
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func dataSourceSecretStubRead(d *schema.ResourceData, meta interface{}) error {
	// id := d.Get("id").(int)
	// secrets, err := server.New(meta.(server.Configuration))

	// if err != nil {
	// 	log.Printf("[DEBUG] configuration error: %s", err)
	// }
	// log.Printf("[DEBUG] getting secret template with id %d", id)

	// stub, err := secrets.SecretStub(id)

	// if err != nil {
	// 	log.Print("[DEBUG] unable to get secret template", err)
	// 	return err
	// }

	// d.SetId(strconv.Itoa(stub.ID))
	// d.Set("name", stub.Name)
	// d.Set("id", stub.ID)
	// d.Set("folderid", stub.FolderID)
	// d.Set("siteid", stub.SiteID)
	// d.Set("secrettemplateid", stub.SecretTemplateID)
	// d.Set("secretpolicyid", stub.SecretPolicyID)
	// d.Set("passwordtypewebscriptid", stub.PasswordTypeWebScriptID)
	// d.Set("launcherconnectassecretid", stub.LauncherConnectAsSecretID)
	// d.Set("checkoutintervalminutes", stub.CheckOutIntervalMinutes)
	// d.Set("active", stub.Active)
	// d.Set("checkedout", stub.CheckedOut)
	// d.Set("checkoutenabled", stub.CheckOutEnabled)
	// d.Set("autochangenabled", stub.AutoChangeEnabled)
	// d.Set("checkoutchangepasswordenabled", stub.CheckOutChangePasswordEnabled)
	// d.Set("delayindexing", stub.DelayIndexing)
	// d.Set("enableinheritpermissions", stub.EnableInheritPermissions)
	// d.Set("enableinheritsecretpolicy", stub.EnableInheritSecretPolicy)
	// d.Set("proxyenabled", stub.ProxyEnabled)
	// d.Set("requirescomment", stub.RequiresComment)
	// d.Set("sessionrecordingenabled", stub.SessionRecordingEnabled)
	// d.Set("weblauncherrequiresincognitomode", stub.WebLauncherRequiresIncognitoMode)

	// var flattenedFields []map[string]interface{}
	// for _, field := range stub.Fields {
	// 	flattenedFields = append(flattenedFields, map[string]interface{}{
	// 		"fieldDescription": field.FieldDescription,
	// 		"fieldId":          field.FieldID,
	// 		"fieldName":        field.FieldName,
	// 		"fileAttachmentId": field.FileAttachmentID,
	// 		"filename":         field.Filename,
	// 		"isfile":           field.IsFile,
	// 		"islist":           field.IsList,
	// 		"isnotes":          field.IsNotes,
	// 		"ispassword":       field.IsPassword,
	// 		"itemValue":        field.ItemValue,
	// 		"listtype":         field.ListType,
	// 		"slug":             field.Slug,
	// 	})
	// }

	// d.Set("fields", flattenedFields)
	return nil
}

func dataSourceSecretStub() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceSecretTemplateRead,

		Schema: map[string]*schema.Schema{
			"fields": {
				Description: "the fields of the secret template",
				Computed:    true,
				Type:        schema.TypeList,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"fielddescription": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"fieldid": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"fieldname": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"fileattachmentid": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"filename": {
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
						"itemvalue": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"listtype": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"slug": {
							Type:     schema.TypeString,
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
				Type:        schema.TypeInt,
			},
			"passwordtypewebscriptid": {
				Description: "the id of the password type webscript",
				Optional:    true,
				Type:        schema.TypeInt,
			},
			"launcherconnectassecretid": {
				Description: "the id of the launcher connect as secret",
				Optional:    true,
				Type:        schema.TypeInt,
			},
			"checkoutintervalminutes": {
				Description: "the secret checkout interval minutes",
				Optional:    true,
				Type:        schema.TypeInt,
			},
			"active": {
				Description: "the secret is enabled or disabled",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"checkedout": {
				Description: "the secret is checked out or not",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"checkoutenabled": {
				Description: "the secret checkout enabled or disabled",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"autochangenabled": {
				Description: "the autochange is enabled or disabled",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"checkoutchangepasswordenabled": {
				Description: "the checkout change password enabled or disabled",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"delayindexing": {
				Description: "the delay indexing is enabled or disabled",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"enableinheritpermissions": {
				Description: "the inherit permission is enabled or disabled",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"enableinheritsecretpolicy": {
				Description: "the inherit secret policy is enabled or disabled",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"proxyenabled": {
				Description: "the proxy enabled or disabled",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"requirescomment": {
				Description: "the comment is required or not",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"sessionrecordingenabled": {
				Description: "the session recording is enabled or disabled",
				Optional:    true,
				Type:        schema.TypeBool,
			},
			"weblauncherrequiresincognitomode": {
				Description: "the secret requires web launcher encognito mode or not",
				Optional:    true,
				Type:        schema.TypeBool,
			},
		},
	}
}
