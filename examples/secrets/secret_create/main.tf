terraform {
  required_version = ">= 1.11.0"
  required_providers {
    tss = {
      source  = "DelineaXPM/tss"
      version = ">= 5.0.0"
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
    itemvalue      = optional(string, "")
    fieldname      = string
    is_password    = optional(bool, false)
    password_value = optional(string)
  }))
  description = "Each entry describes one template field. For password fields, set is_password = true and supply password_value instead of itemvalue — the value is write-only and will not be written to Terraform state."
  sensitive   = true
}

variable "password_wo_version" {
  type        = number
  description = "Rotation trigger for write-only password fields. Bump this (to any new integer) to signal that password_value has changed and must be re-sent to TSS on the next apply."
  default     = 1
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
  name             = var.tss_secret_name
  folderid         = var.tss_secret_folderid
  siteid           = var.tss_secret_siteid
  secrettemplateid = var.tss_secret_templateid
  active           = true
  dynamic "fields" {
    for_each = var.fields
    content {
      fieldname = fields.value.fieldname
      # Password fields: send via write-only password_value (never lands in state).
      # SSH key fields when generating: leave null so the server fills in.
      # Everything else: pass through itemvalue unchanged.
      itemvalue = (
        fields.value.is_password ? null :
        (var.generate_ssh_keys && contains(var.ssh_key_fields, fields.value.fieldname)) ? null :
        fields.value.itemvalue
      )
      password_value      = fields.value.is_password ? fields.value.password_value : null
      password_wo_version = fields.value.is_password ? var.password_wo_version : null
    }
  }

  sshkeyargs {
    generatepassphrase = var.generate_passphrase
    generatesshkeys    = var.generate_ssh_keys
  }
}
