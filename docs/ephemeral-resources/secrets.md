---
page_title: "tss_secrets Ephemeral Resource - terraform-provider-tss"
subcategory: ""
description: |-
  Reads one field from several Secret Server secrets at runtime without storing the values in Terraform state.
---

# tss_secrets (Ephemeral Resource)

Fetches the same field from a list of secrets, with bounded concurrent Secret Server requests, without writing the values to Terraform state or plan files. Every requested ID keeps its position in `secrets`.

## Example Usage

```terraform
ephemeral "tss_secrets" "db_passwords" {
  ids   = [1234, 1235]
  field = "password"
}

provider "echo" {
  data = join(",", [for s in ephemeral.tss_secrets.db_passwords.secrets : s.value])
}
```

A missing or misspelled field aborts the read, and so does any fetch failure for any requested ID: a shortened, re-indexed result could feed the wrong value to a downstream consumer. Remove IDs that no longer exist from `ids`, or split the read so one deletable ID is not fatal to the batch.

## Schema

### Required

- `field` (String) The field to extract from the secrets.
- `ids` (List of Number) A list of IDs of the secrets.

### Read-Only

- `secrets` (Attributes List) A list of secrets with their field values, in the same order as `ids` (see [below for nested schema](#nestedatt--secrets)).

<a id="nestedatt--secrets"></a>
### Nested Schema for `secrets`

Read-Only:

- `id` (Number) The ID of the secret.
- `value` (String, Sensitive) The ephemeral value of the field of the secret.
