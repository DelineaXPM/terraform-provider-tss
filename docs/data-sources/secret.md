---
page_title: "tss_secret Data Source - terraform-provider-tss"
subcategory: ""
description: |-
  Reads one field of a Secret Server secret into Terraform state.
---

# tss_secret (Data Source)

Reads one field of a secret from Secret Server. The value is stored in Terraform state (marked sensitive); to read a secret without persisting the value, use the [`tss_secret`](../ephemeral-resources/secret.md) ephemeral resource instead.

## Example Usage

```terraform
data "tss_secret" "db_password" {
  id    = "1234"
  field = "password"
}

output "db_password" {
  value     = data.tss_secret.db_password.value
  sensitive = true
}
```

`field` matches the Secret Server field by slug (`"password"`, `"username"`, …) or by display name, case-insensitively. A missing or misspelled field aborts the read. Any fetch failure, including HTTP 404, also aborts the read because the SDK cannot distinguish a missing secret from a missing attachment on a live secret; classic Secret Server's ambiguous HTTP 400 response is reported with guidance to verify access or remove a deleted ID.

## Schema

### Required

- `field` (String) The field to extract from the secret.
- `id` (String) The ID of the secret to retrieve.

### Read-Only

- `value` (String, Sensitive) The value of the requested field from the secret.
