# Thycotic Secret Server - Terraform Provider

The [Thycotic](https://thycotic.com/) [Secret Server](https://thycotic.com/products/secret-server/) [Terraform](https://www.terraform.io/) Provider allows you to access and reference Secrets in your vault for use in Terraform configurations.

## Install via Registry

> Preferred way to install

The latest release can be [downloaded from the terraform registry](https://registry.terraform.io/providers/thycotic/tss/latest). The documentation can be found [here](https://registry.terraform.io/providers/thycotic/tss/latest/docs).

If wish to install straight from source, follow the steps below.

## Install form Source

### Terraform 0.12 and earlier

Extract the specific file for your OS and Architecture to the plugins directory
of the user's profile. You may have to create the directory.

| OS      | Default Path                    |
| ------- | ------------------------------- |
| Linux   | `~/.terraform.d/plugins`        |
| Windows | `%APPDATA%\terraform.d\plugins` |

### Terraform 0.13 and later

Terraform 0.13 uses a different file system layout for 3rd party providers. More information on this can be found [here](https://www.terraform.io/upgrade-guides/0-13.html#new-filesystem-layout-for-local-copies-of-providers). The following folder path will need to be created in the plugins directory of the user's profile.

#### Windows

```text
%APPDATA%\TERRAFORM.D\PLUGINS
└───terraform.thycotic.com
    └───thycotic
        └───tss
            └───1.0.2
                └───windows_amd64
```

#### Linux

```text
~/.terraform.d/plugins
└───terraform.thycotic.com
    └───thycotic
        └───tss
            └───1.0.2
                ├───linux_amd64
```

## Usage

For Terraform 0.13+, include the `terraform` block in your configuration, or plan, that specifies the provider:

```terraform
terraform {
  required_providers {
    tss = {
      source = "thycotic/tss"
      version = "1.0.2"
    }
  }
}
```

To run the example, create a `terraform.tfvars`:

```json
tss_username   = "my_app_user"
tss_password   = "Passw0rd."
tss_server_url = "https://example/SecretServer"
tss_secret_id  = "1"
```
## Environment variables

You can provide your credentials via the TSS_SERVER_URL, TSS_USERNAME and TSS_PASSWORD environment variables.
In this case, tss provider could be represented like this 
```
provider "tss" {}
```
Usage
```
$ export TSS_USERNAME="my_app_user"
$ export TSS_PASSWORD="Passw0rd."
$ export TSS_SERVER_URL="https://localhost/SecretServer"
$ terraform plan
```