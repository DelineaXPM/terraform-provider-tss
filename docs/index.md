---
page_title: "Delinea Secret Server - Terraform Provider"
subcategory: ""
description: |-
  
---

# Delinea Secret Server - Terraform Provider

The [Delinea](https://delinea.com/) [Secret Server](https://delinea.com/products/secret-server/) [Terraform](https://www.terraform.io/) Provider allows you to access and reference Secrets in your vault for use in Terraform configurations.

For Delinea's official Terraform documentation, please see [here](https://docs.delinea.com/online-help/integrations/terraform/index.htm) for more information.

## Example Usage

This provider requires Terraform 1.11 or later. Include the `terraform` block in your configuration or plan to specify the provider and Terraform version:

```terraform
terraform {
  required_version = ">= 1.11.0"
  required_providers {
    tss = {
      source  = "DelineaXPM/tss"
      version = ">= 5.0.0"
    }
  }
}
```

Configure the provider directly, or leave the block empty and use the environment variables below:

```terraform
provider "tss" {
  server_url = "https://localhost/SecretServer"
  username   = var.tss_username
  password   = var.tss_password
}
```

Standalone example configurations for every resource, data source, and ephemeral resource live under `examples/secrets/<name>/` in the provider repository, with matching variable files under `vars/secrets/`.

## Environment variables

Each provider attribute resolves in the order *explicit provider attribute > environment variable > unset*. With the env vars exported, the provider block can be empty:

| Env var                   | Provider attribute    |
|---------------------------|-----------------------|
| `TSS_SERVER_URL`          | `server_url`          |
| `TSS_USERNAME`            | `username`            |
| `TSS_PASSWORD`            | `password`            |
| `TSS_TOKEN`               | `token`               |
| `TSS_DOMAIN`              | `domain`              |
| `TSS_ALLOW_INSECURE_HTTP` | `allow_insecure_http` |

```terraform
provider "tss" {}
```

Username/password:

```shell
$ export TSS_SERVER_URL="https://localhost/SecretServer"
$ export TSS_USERNAME="my_app_user"
$ export TSS_PASSWORD="Passw0rd."
$ terraform plan
```

OAuth token instead of username/password:

```shell
$ export TSS_SERVER_URL="https://localhost/SecretServer"
$ export TSS_TOKEN="PASTE_TOKEN_HERE"
$ terraform plan
```

After the env-var fallback runs, the provider enforces that `server_url` is set and that exactly one of `(username + password)` or `token` is set.

`server_url` must use `https://` for any remote host. A plaintext `http://` URL to a non-loopback host is rejected when the provider is configured — plan and apply fail immediately — because it would expose the credential on the wire; `http://` remains usable only for loopback addresses such as a local test server. To permit remote plaintext HTTP deliberately, set `allow_insecure_http = true` (or `TSS_ALLOW_INSECURE_HTTP=true`).

Username/password authentication probes the configured origin before sending credentials so the SDK can distinguish Secret Server from Delinea Platform. The reverse proxy must allow an unauthenticated `GET` to `<server_url>/api/v1/healthcheck` for Secret Server (or `<server_url>/health` for Platform) to return a direct 2xx healthy response; redirects are deliberately not followed. Static Secret Server token authentication skips this probe. This is new in v5.0.0, so update proxy routing before upgrading if those health paths are currently blocked or redirected.

## Schema

### Optional

- `server_url` (String) The Secret Server base URL e.g. https://localhost/SecretServer. May also be supplied via the `TSS_SERVER_URL` environment variable.
- `username` (String) The username of the Secret Server User to connect as. May also be supplied via the `TSS_USERNAME` environment variable.
- `password` (String, Sensitive) The password of the Secret Server User. May also be supplied via the `TSS_PASSWORD` environment variable.
- `token` (String, Sensitive) A token to authenticate the Secret Server User, used instead of username and password. May also be supplied via the `TSS_TOKEN` environment variable.
- `domain` (String) Domain of the Secret Server user, for directory-backed accounts. May also be supplied via the `TSS_DOMAIN` environment variable.
- `allow_insecure_http` (Boolean) Permit a plaintext `http://` server URL to a non-loopback host. Defaults to `false`; plaintext HTTP exposes the credential on the wire, so set this only after accepting that risk. May also be supplied via the `TSS_ALLOW_INSECURE_HTTP` environment variable.
