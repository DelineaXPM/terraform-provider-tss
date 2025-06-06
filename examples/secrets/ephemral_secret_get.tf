terraform {
  required_version = "1.12.1"
  required_providers {
    tss = {
      source = "DelineaXPM/tss"
      version = "2.1.0"
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

variable "tss_secret_id" {
  type = string
}

provider "tss" {
  username   = var.tss_username
  password   = var.tss_password
  server_url = var.tss_server_url
}

ephemeral "tss_secret" "my_username" {
  id    = var.tss_secret_id
  field = "username"
}

ephemeral "tss_secret" "my_password" {
  id    = var.tss_secret_id
  field = "password"
}
