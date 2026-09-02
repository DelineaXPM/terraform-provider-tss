---
page_title: "tss_secret_deletion Resource - terraform-provider-tss"
subcategory: ""
description: |-
  One-shot deletion of a Secret Server secret by ID, whether or not Terraform manages it.
---

# tss_secret_deletion (Resource)

A one-shot resource that deletes a secret by ID without requiring it to be in Terraform state. The deletion happens when the resource is created (the `Creating...` line in `terraform apply` output is the deletion). The completed operation stays in state so a later apply does not delete a restored secret again; destroying the resource only removes the record.

Changing `secret_id` replaces the operation and deletes the newly selected secret.

## Example Usage

```terraform
resource "tss_secret_deletion" "retired" {
  secret_id = 12345
}

resource "tss_secret_deletion" "batch" {
  for_each  = toset(["1001", "1002", "1003"])
  secret_id = tonumber(each.key)
}
```

## Behavior on refresh

- A secret Secret Server no longer returns (the classic HTTP 400 access-denied response, or a Platform 404) keeps the completed record without a warning.
- A secret that is readable but inactive (a recycled secret on Platform vaults) keeps the record.
- A secret that is readable and active produces a warning. Terraform keeps the record and does not delete the secret again automatically; remove the resource from state and apply again if another deletion is intended.
- Authentication, authorization, network, and server errors fail the refresh.

## Behavior on failure

If Secret Server refuses the DELETE, the provider reads the secret back to classify the failure: a readable, active secret is reported as "Secret Deletion Refused" (checked out, DoubleLocked, or missing delete permission), while an unreadable or inactive secret is reported as "Secret Deletion Unverified" because the responses are consistent with both an already-deleted secret and missing permission.

## Schema

### Required

- `secret_id` (Number) The ID of the secret to delete. Changing it replaces the operation and deletes the newly selected secret.

### Read-Only

- `id` (String) The ID of the resource. This is set to `secret_<secret_id>` after deletion.
