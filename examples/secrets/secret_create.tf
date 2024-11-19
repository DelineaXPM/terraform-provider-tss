terraform {
  required_version = "1.5.2"
  required_providers {
    tss = {
      source = "DelineaXPM/tss"
      version = "2.0.9"
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
    itemvalue = string
	fieldname = string
  }))
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
      fieldname   = fields.value.fieldname
      itemvalue = fields.value.itemvalue
    }
  }
}