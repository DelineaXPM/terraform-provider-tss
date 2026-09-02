---
page_title: "tss_resource_secret Resource - terraform-provider-tss"
subcategory: ""
description: |-
  Creates and manages a secret in Delinea Secret Server.
---

# tss_resource_secret (Resource)

Creates and manages a secret in Delinea Secret Server. Field values are declared with `fields` blocks whose `fieldname` matches a display name or slug on the selected template.

## Example Usage

```terraform
resource "tss_resource_secret" "example" {
  name             = "example"
  folderid         = "5"
  siteid           = "1"
  secrettemplateid = "2"

  fields {
    fieldname = "Username"
    itemvalue = "myuser"
  }
  fields {
    fieldname           = "Password"
    password_value      = var.db_password
    password_wo_version = 1
  }
}
```

`folderid`, `siteid`, and `secrettemplateid` are strings that hold integer IDs. Find the template ID in the Secret Server URL when opening the template under Admin → Secret Templates, and the field names or slugs on the template's Fields tab. Declare one `fields` block per template field you want to manage; fields you leave out keep whatever value Secret Server holds.

### Password fields (write-only, Terraform 1.11+)

For any field whose template marks `IsPassword`, use `password_value` instead of `itemvalue`. `password_value` is write-only — Terraform never writes it to state — and must be paired with `password_wo_version`, which acts as the rotation trigger. To rotate, change `password_value` and bump `password_wo_version` (any new integer) in the same apply.

Password fields do not accept `itemvalue`: state never holds a password, so a configured `itemvalue` could never match the value Terraform reads back, and the provider rejects it with a message that names this migration. Existing v4 configurations that still pass a password through `itemvalue` must move it to `password_value` + `password_wo_version` before applying with v5.

Alternatively set `generate = true` (with `password_wo_version`) to have Secret Server generate a password from the template's password-requirement policy; bumping `password_wo_version` requests a new generated password.

The provider rejects these invalid field configurations before writing the secret:

- A `fieldname` that does not match a display name or slug on the selected template.
- A `fieldname` that ambiguously matches one field's display name and another field's slug.
- `itemvalue` on a field the template marks as a password.
- `password_value` or `generate = true` on a field the template does not mark as a password.
- An empty `password_value`.
- `password_value` without `password_wo_version` in the same field block.
- A configured `filename` on a field the template does not mark as a file, or a filename whose extension contains characters other than ASCII letters, digits, or underscores.
- A filename change on an existing zero-byte attachment without also supplying `itemvalue`; Secret Server interprets an empty file update as deletion, so the content must be supplied again when renaming it.
- A new or changed `password_wo_version` without `password_value` or `generate = true` in the same field block.
- Any combination of `password_value`, `generate = true`, and non-empty `itemvalue` on the same field.

Switching an existing field between `generate = true` and an explicit `password_value` is itself a password request and does not require changing the configured `password_wo_version`. Subsequent changes to the same password mode require a version bump.

### Secret settings you do not configure

Every optional secret-level setting (`active`, `checkoutenabled`, `requirescomment`, `enableinheritpermissions`, `secretpolicyid`, and so on) is also computed. When your configuration omits one, the provider preserves whatever value Secret Server holds across updates, so an administrator's or policy's settings survive a Terraform-driven rename or field change. Configure a setting explicitly when Terraform should own it.

### Reads fail closed

If Secret Server reports the secret as inaccessible during refresh, the provider keeps the resource in state and returns an error instead of silently planning a replacement. Classic Secret Server answers both "deleted" and "no permission" with the same HTTP 400, and a 404 can come from an attachment download on a live secret, so neither response proves the secret is gone. When you have confirmed a secret was deleted outside Terraform, run `terraform state rm` on the resource and apply again.

### SSH key generation

Set the `sshkeyargs` block when the template supports SSH key generation. Secret Server generates keys only while creating a secret, so changing the block replaces the secret. Leave the generated fields' `itemvalue` unset; the provider records the generated values after apply and preserves them across later updates.

## Schema

### Required

- `folderid` (String) The folder ID of the secret.
- `name` (String) The name of the secret.
- `secrettemplateid` (String) The template ID in which the secret will be created.
- `siteid` (String) The site ID where the secret will be created.

### Optional

- `active` (Boolean) Whether the secret is active. New secrets default to active when omitted; omitted values are preserved across updates.
- `autochangenabled` (Boolean) Whether auto-change is enabled for the secret. Preserved across updates when omitted.
- `checkedout` (Boolean) Whether the secret is checked out. Preserved across updates when omitted.
- `checkoutchangepasswordenabled` (Boolean) Whether checkout change password is enabled. Preserved across updates when omitted.
- `checkoutenabled` (Boolean) Whether checkout is enabled for the secret. Preserved across updates when omitted.
- `checkoutintervalminutes` (Number) The checkout interval in minutes. Preserved across updates when omitted.
- `delayindexing` (Boolean) Whether delay indexing is enabled. Preserved across updates when omitted.
- `enableinheritpermissions` (Boolean) Whether inherit permissions is enabled. Preserved across updates when omitted.
- `enableinheritsecretpolicy` (Boolean) Whether inherit secret policy is enabled. Preserved across updates when omitted.
- `fields` (Block List) The fields of the secret, one block per template field to manage (see [below for nested schema](#nestedblock--fields)).
- `launcherconnectassecretid` (Number) The ID of the launcher connect-as secret. Preserved across updates when omitted.
- `passwordtypewebscriptid` (Number) The ID of the password type web script. Preserved across updates when omitted.
- `proxyenabled` (Boolean) Whether proxy is enabled. Preserved across updates when omitted.
- `requirescomment` (Boolean) Whether a comment is required. Preserved across updates when omitted.
- `secretpolicyid` (Number) The ID of the secret policy. Preserved across updates when omitted.
- `sessionrecordingenabled` (Boolean) Whether session recording is enabled. Preserved across updates when omitted.
- `sshkeyargs` (Block) SSH key generation arguments used during creation. Changing this block replaces the secret (see [below for nested schema](#nestedblock--sshkeyargs)).
- `weblauncherrequiresincognitomode` (Boolean) Whether the web launcher requires incognito mode. Preserved across updates when omitted.

### Read-Only

- `id` (Number) The ID of the secret.

<a id="nestedblock--fields"></a>
### Nested Schema for `fields`

Optional:

- `filename` (String) Filename for a file-type field. Configured filenames must end in an extension containing only ASCII letters, digits, or underscores; omit this attribute to use Secret Server's `File.txt` default.
- `generate` (Boolean) Request server-side password generation from the template's password-requirement policy. Only honored on fields the template marks as password fields. Mutually exclusive with `password_value` and `itemvalue`. Pair with `password_wo_version` to rotate.
- `itemvalue` (String, Sensitive) The value of a non-password field. Password fields reject it; use `password_value` instead. For SSH key generation, leave it unset so the server-generated value is recorded; setting an SSH file field explicitly to an empty string later removes that attachment.
- `password_value` (String, Sensitive, Write-only) Non-empty password value for password fields. Never stored in Terraform state. Requires Terraform 1.11+ and must be paired with `password_wo_version` so Terraform can detect later rotations.
- `password_wo_version` (Number) Rotation trigger for `password_value` or `generate`. Bump this integer to signal Terraform to re-send `password_value` to Secret Server, or to ask for a new generated password when `generate=true`, on the next apply.

Read-Only:

- `fielddescription` (String) Field description from the template. Populated after apply; do not set in config.
- `fieldid` (Number) Secret Server template field ID; stable per template, shared across every secret that uses the template. Populated after apply; do not set in config.
- `fileattachmentid` (Number) Server-assigned attachment ID for a file-type field. Populated after apply; do not set in config.
- `itemid` (Number) Server-assigned database ID of this field-value record. Populated after apply; do not set in config.
- `isfile` (Boolean) Whether the template defines this as a file field.
- `islist` (Boolean) Whether the template defines this as a list field, when returned by Secret Server.
- `isnotes` (Boolean) Whether the template defines this as a notes field.
- `ispassword` (Boolean) Whether the template defines this as a password field.
- `listtype` (String) The template list type, when returned by Secret Server.
- `slug` (String) Field's URL slug, assigned by the template. Populated after apply; do not set in config.

Required:

- `fieldname` (String) The template field's display name or slug (case-insensitive).

<a id="nestedblock--sshkeyargs"></a>
### Nested Schema for `sshkeyargs`

Optional:

- `generatepassphrase` (Boolean) Whether to generate a passphrase for the SSH key.
- `generatesshkeys` (Boolean) Whether to generate SSH keys.

## Computed fields

Each `fields` block has attributes that Secret Server or its template assigns automatically after `terraform apply`. They appear in Terraform state but are not user-settable: `itemid`, `fieldid`, `fileattachmentid`, `slug`, `fielddescription`, `isfile`, `islist`, `isnotes`, `ispassword`, and `listtype`. Setting any of them in your config produces a plan error ("Can't configure a value for `itemid`: its value will be decided automatically based on the result of applying this configuration").
