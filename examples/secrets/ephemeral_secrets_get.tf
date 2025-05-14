terraform {
  required_version = "1.5.2"
  required_providers {
    tss = {
      source = "DelineaXPM/tss"
      version = "2.0.10"
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

ephemeral "tss_secrets" "my_usernames" {
  ids    = var.tss_secret_ids
  field = "username"
}

ephemeral "tss_secrets" "my_passwords" {
  ids    = var.tss_secret_ids
  field = "password"
}