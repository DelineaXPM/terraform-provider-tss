---
page_title: "tss_secret Ephemeral Resource - terraform-provider-tss"
subcategory: ""
description: |-
  Reads one field of a Secret Server secret at runtime without storing it in Terraform state.
---

# tss_secret (Ephemeral Resource)

Fetches one field of a secret from Secret Server during plan and apply without writing the value to Terraform state or plan files. Use it for values that must only exist during a run, such as a database password handed to another provider's configuration.

## Example Usage

```terraform
ephemeral "tss_secret" "db_password" {
  id    = "1234"
  field = "password"
}

provider "postgresql" {
  password = ephemeral.tss_secret.db_password.value
}
```

Ephemeral values can only flow into other ephemeral contexts: provider configuration, provisioner and connection blocks, write-only arguments, and ephemeral variables or outputs. `field` matches by slug or display name, case-insensitively; a missing field or any fetch failure aborts the read. Standalone examples live under `examples/secrets/ephemeral_secret_get` and `examples/secrets/ephemeral_secrets_get` in the provider repository.

## Schema

### Required

- `field` (String) The field to extract from the secret.
- `id` (String) The ID of the secret to retrieve.

### Read-Only

- `value` (String, Sensitive) The value of the requested field from the secret.
