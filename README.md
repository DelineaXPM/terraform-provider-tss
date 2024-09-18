# Delinea Secret Server - Terraform Provider

The [Delinea](https://delinea.com/) [Secret Server](https://delinea.com/products/secret-server/) [Terraform](https://www.terraform.io/) Provider allows you to access and reference Secrets in your vault for use in Terraform configurations. Detail documentation of this integration is available [here](https://docs.delinea.com/online-help/integrations/terraform/configure.htm)

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
            └───2.0.8
                └───windows_amd64
```

#### Linux

```text
~/.terraform.d/plugins
└───terraform.delinea.com
    DelineaXPM
        └───tss
            └───2.0.8
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

If you want to access the secret server via the platform, provide the platform URL as an input to tss_server_url, and the platform user credentials to tss_username and tss_password.

Get Secret By ID:

```hcl
tss_username   = "my_app_user"
tss_password   = "Passw0rd."
tss_server_url = "https://example/SecretServer"
tss_secret_id  = "1"
```
Get Secrets By ID:

```hcl
tss_username   = "my_app_user"
tss_password   = "Passw0rd."
tss_server_url = "https://example/SecretServer"
tss_secret_ids  = ["1", "2", "3"]
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

Deactivate Secret:

The secret will be deactivated in the Secret Server when the `terraform destroy` command is executed.

## Environment variables

You can provide your credentials via the tss_server_url, tss_username and tss_password environment variables.
In this case, tss provider could be represented like this 
```
provider "tss" {
  username   = var.tss_username
  password   = var.tss_password
  server_url = var.tss_server_url
}
```
Usage (For Linux)
```
$ export TF_VAR_tss_username="my_app_user"
$ export TF_VAR_tss_password="Passw0rd."
$ export TF_VAR_tss_server_url="https://localhost/SecretServer"
$ terraform plan or $ terraform apply
```
Usage (For Windows)
```
> set TF_VAR_tss_username="my_app_user"
> set TF_VAR_tss_password="Passw0rd."
> set TF_VAR_tss_server_url="https://localhost/SecretServer"
> terraform plan or > terraform apply
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

## Encrypt terraform state file using script wrapper

Terraform supports multiple backends to securely store state files, such as AWS S3, Azure Blob Storage, and others. These backends also include built-in state locking mechanisms. However, when storing state files on a local machine drive, you need to manually encrypt the state file data to keep it secure.

To encrypt or decrypt state file data during the Terraform workflow, you must perform encryption before executing Terraform commands and decryption afterward. This can be achieved by creating script wrappers around Terraform commands like terraform init, terraform apply, and terraform destroy.

To use these script wrappers, place the script files in the Terraform executable directory and set the required user credentials in environment variables. For instructions on setting environment variables, refer to the section titled "Environment Variables" above.

Scripts for reference and more detailed information are available [here](https://docs.delinea.com/online-help/integrations/terraform/index.htm). You can modify file paths in these scripts as needed.

You can then execute the script wrappers as shown below:

Usage (For Linux)
```
$ sh terraform_init.sh
$ sh terraform_apply.sh
$ sh terraform_destroy.sh
```
Usage (For Windows)
```
> terraform_init.bat
> terraform_apply.bat
> terraform_destroy.bat
```