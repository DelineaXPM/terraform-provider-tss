terraform {
  required_version = "1.12.1"
  required_providers {
    tss = {
      source = "DelineaXPM/tss"
      version = "3.0.0"
    }
  }
}

variable "tss_username" {
  type = string
}

variable "tss_password" {
  type = string
}

variable "tss_server_url" {
  type = string
}

variable "tss_secret_name" {
  type = string
}

variable "tss_secret_siteid" {
  type = string
}

variable "tss_secret_folderid" {
  type = string
}

variable "tss_secret_templateid" {
  type = string
}

variable "fields" {
  type = list(object({
    itemvalue = optional(string, "")
    fieldname = string
  }))
}

variable "ssh_key_fields" {
  type        = list(string)
  description = "List of field names that should be generated when SSH key generation is enabled"
  default     = ["Public Key", "Private Key", "Private Key Passphrase"]
}

variable "generate_passphrase" {
  type        = bool
  description = "Whether to generate a passphrase for the SSH key"
  default     = false
}

variable "generate_ssh_keys" {
  type        = bool
  description = "Whether to generate SSH keys"
  default     = false
}

provider "tss" {
  username   = var.tss_username
  password   = var.tss_password
  server_url = var.tss_server_url
}

resource "tss_resource_secret" "secret_name" {
  name = var.tss_secret_name
  folderid = var.tss_secret_folderid
  siteid = var.tss_secret_siteid
  secrettemplateid = var.tss_secret_templateid
  active = true
  dynamic "fields" {
    for_each = var.fields
    content {
      fieldname = fields.value.fieldname
      # Only set itemvalue if SSH key generation is disabled OR if this is not an SSH key field
      itemvalue = (var.generate_ssh_keys && contains(var.ssh_key_fields, fields.value.fieldname)) ? null : fields.value.itemvalue
    }
  }
  
  sshkeyargs {
    generatepassphrase = var.generate_passphrase
    generatesshkeys    = var.generate_ssh_keys
  }
}