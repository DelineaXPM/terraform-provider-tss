---
page_title: "tss_secret Ephemeral - terraform-provider-tss"
subcategory: ""
description: |-
  
---

# tss_secret (Ephemeral)

## Schema

### Required

- `id` (String) The ID of a secret
- `field` (String) The field to retrieve within a secret

### Read-Only

- `value` (String) The retrieved field from the identified secret

## Example Usage

To use ephemeral resources in your Terraform configuration, define them using the ephemeral block type. For instance:

```hcl
ephemeral "tss_secret" "my_username" {
  id    = var.tss_secret_id
  field = "username"
}
```

Note: Sample Terraform files demonstrating the use of ephemeral resources are available in the terraform-provider-tss/examples/secrets directory for reference.

This enhancement is particularly valuable in dynamic infrastructure environments where secrets must be accessed securely and temporarily during provisioning.
