terraform {
  required_version = "1.6.3"
  required_providers {
    tss = {
      source = "terraform.delinea.com/delinea/tss"
      version = "2.0.2"
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
    field_id   = number
    item_value = string
  }))
}

provider "tss" {
  username   = var.tss_username
  password   = var.tss_password
  server_url = var.tss_server_url
}


resource "tss_secret" "secret_name" {
  name = var.tss_secret_name
  folder_id = var.tss_secret_folderid
  siteid = var.tss_secret_siteid
  secret_template_id = var.tss_secret_templateid
  active = true
  dynamic "fields" {
    for_each = var.fields
    content {
      field_id   = fields.value.field_id
      item_value = fields.value.item_value
    }
  }
}