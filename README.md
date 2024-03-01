# Delinea Secret Server - Terraform Provider

The [Delinea](https://delinea.com/) [Secret Server](https://delinea.com/products/secret-server/) [Terraform](https://www.terraform.io/) Provider allows you to access and reference Secrets in your vault for use in Terraform configurations.

## Install via Registry

> Preferred way to install

The latest release can be [downloaded from the terraform registry](https://registry.terraform.io/providers/DelineaXPM/tss/latest). The documentation can be found [here](https://registry.terraform.io/providers/DelineaXPM/tss/latest/docs).

If wish to install straight from source, follow the steps below.

## Install form Source

### Terraform 0.13 and later

Terraform 0.13 uses a different file system layout for 3rd party providers. More information on this can be found [here](https://www.terraform.io/upgrade-guides/0-13.html#new-filesystem-layout-for-local-copies-of-providers). The following folder path will need to be created in the plugins directory of the user's profile.

#### Windows

```text
%APPDATA%\TERRAFORM.D\PLUGINS
└───terraform.delinea.com
    DelineaXPM
        └───tss
            └───2.0.6
                └───windows_amd64
```

#### Linux

```text
~/.terraform.d/plugins
└───terraform.delinea.com
    DelineaXPM
        └───tss
            └───2.0.6
                ├───linux_amd64
```

## Usage

For Terraform 0.13+, include the `terraform` block in your configuration, or plan, that specifies the provider:

```terraform
terraform {
  required_providers {
    tss = {
      source = "DelineaXPM/tss"
      version = "~> 2.0"
    }
  }
}
```

To run the example, create a `terraform.tfvars` and use below variables to get and create/update secret:

Get Secret By ID:

```hcl
tss_username   = "my_app_user"
tss_password   = "Passw0rd."
tss_server_url = "https://example/SecretServer"
tss_secret_id  = "1"
```
Create/Update Secret:

```hcl
tss_username   = "my_app_user"
tss_password   = "Passw0rd."
tss_server_url = "https://example/SecretServer"
tss_secret_name = "Windows Account"
tss_secret_siteid = 1
tss_secret_folderid = 0
tss_secret_templateid = 6003
fields = [
  {
    fieldname   = "Machine"
    itemvalue = "hostname/ip"
  },
  {
    fieldname   = "Username"
    itemvalue = "my_app_user"
  },
  {
    fieldname   = "Password"
    itemvalue = "Passw0rd."
  },
  {
    fieldname   = "Notes"
    itemvalue = ""
  }
]
```
Above Create/Update Secret variables are for Windows Account secret template of secret server. To create variables to support different secret template refer below steps

1. Open secret templete in secret server Admin => Secret Templates
2. Replace tss_secret_templateid value with the teplate id which can be see in the URL when user click on any template in secret server.
3. Click on Fields tab
4. Based on template fields add/update field (with field name and item value) in fields array as above example. In above example there are four fields but in other template
 there might be more/less flieds. Accordingly, add/remove field entry from the fields array.

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

## Domain user accounts

Domain users, such as Active Directory accounts, can be used by supplying the `tss_domain` parameter. E.G.

```hcl
tss_username   = "my_app_user"
tss_password   = "Passw0rd."
tss_server_url = "https://example/SecretServer"
tss_domain     = "mycompany.com"
```

Alternatively, the domain can be provided with an environment variable:

```sh
$ export TSS_DOMAIN="mycompany.com"
```
