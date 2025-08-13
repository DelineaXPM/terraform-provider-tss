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

variable "tss_secret_id" {
  type = string
}

provider "tss" {
  username   = var.tss_username
  password   = var.tss_password
  server_url = var.tss_server_url
}

resource "tss_secret_deletion" "delete_secret" {
  secret_id = var.tss_secret_id
}
