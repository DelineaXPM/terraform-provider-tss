terraform {
  required_version = "1.5.2"
  required_providers {
    tss = {
      source = "DelineaXPM/tss"
      version = "2.0.8"
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

variable "tss_secret_ids" {
  type = list(string)
}

provider "tss" {
  username   = var.tss_username
  password   = var.tss_password
  server_url = var.tss_server_url
}

data "tss_secrets" "my_usernames" {
  ids    = var.tss_secret_ids
  field = "username"
}

data "tss_secrets" "my_passwords" {
  ids    = var.tss_secret_ids
  field = "password"
}

output "usernames" {
  value = [for secret in data.tss_secrets.my_usernames.secrets : {
    id    = secret.id
    value = secret.value
  }]
}

output "passwords" {
  value = [for secret in data.tss_secrets.my_passwords.secrets : {
    id    = secret.id
    value = secret.value
  }]
}
