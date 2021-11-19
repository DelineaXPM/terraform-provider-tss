terraform {
  required_version = ">= 0.12.20"
  required_providers {
    tss = {
      source  = "thycotic/tss"
      version = "1.0.2"
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

data "tss_secret" "my_username" {
  id    = var.tss_secret_id
  field = "username"
}

data "tss_secret" "my_password" {
  id    = var.tss_secret_id
  field = "password"
}

output "username" {
  value     = data.tss_secret.my_username.value
}

output "password" {
  value     = data.tss_secret.my_password.value
}
