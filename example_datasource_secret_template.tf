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

variable "tss_template_id" {
  type = string
}

provider "tss" {
  username   = var.tss_username
  password   = var.tss_password
  server_url = var.tss_server_url
}

data "tss_secret_template" "my_template" {
  id    = var.tss_template_id
}

output "template_fields" {
  fields     = data.tss_secret_template.my_template.fields
}

output "template_name" {
  name     = data.tss_secret_template.my_template.name
}

