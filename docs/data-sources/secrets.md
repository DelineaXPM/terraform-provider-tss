---
page_title: "tss_secrets Data Source - terraform-provider-tss"
subcategory: ""
description: |-
  Read a single field from multiple secrets with bounded concurrent requests.
  The list of secret IDs is supplied via the `ids` attribute and the field to
  extract is identified by its slug or name via `field`.
---

# tss_secrets (Data Source)

Read a single field from multiple secrets with bounded concurrent Secret Server requests. Useful when several Terraform resources need values from a batch of secrets — preferable to declaring N separate `tss_secret` data sources when a single typed result list is easier to consume.

For reading a single secret, use the [`tss_secret`](secret.md) data source. For reading secrets without persisting the values to state, use the [`tss_secret`](../ephemeral-resources/secret.md) or [`tss_secrets`](../ephemeral-resources/secrets.md) ephemeral resources.

## Example Usage

```terraform
data "tss_secrets" "db_passwords" {
  ids   = [1234, 1235, 1236]
  field = "password"
}

# Look up a single result by ID:
output "primary_db_password" {
  value     = [for s in data.tss_secrets.db_passwords.secrets : s.value if s.id == 1234][0]
  sensitive = true
}
```

The `field` argument matches a Secret Server field by its **slug** (the URL-style name; `"password"`, `"username"`, etc.) or by its display name. Match is case-insensitive. The same field name is read from every secret in `ids`, and every requested ID retains its position in `secrets`. A missing or misspelled field aborts the read instead of returning an empty credential. Any secret-fetch failure, including HTTP 404, also aborts the read because the SDK cannot distinguish a missing secret from a missing attachment on a live secret. Classic Secret Server's ambiguous HTTP 400 response includes guidance to verify access or remove a deleted ID from the configuration.

## Schema

### Required

- `ids` (List of Number) A list of IDs of the secrets to read.
- `field` (String) The slug or display name of the field to read from each secret. Same name applied across every ID in `ids`.

### Read-Only

- `secrets` (Attributes List) The list of `{id, value}` results, in the same order as `ids`. The `value` attribute is marked sensitive — Terraform CLI output masks it and `terraform show -json` emits the value but it is treated as sensitive. (See [Nested Schema for `secrets`](#nestedatt--secrets).)

<a id="nestedatt--secrets"></a>
### Nested Schema for `secrets`

Read-Only:

- `id` (Number) The Secret Server secret ID, mirroring the corresponding entry in the input `ids`.
- `value` (String, Sensitive) The value of `field` on this secret, retrieved from Secret Server.
