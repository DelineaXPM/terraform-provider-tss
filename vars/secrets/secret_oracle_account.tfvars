tss_username          = "username"
tss_password          = "password"
tss_server_url        = "https://secretserver.com"
tss_secret_name       = "Database Account"
tss_secret_siteid     = 1
tss_secret_folderid   = -1
tss_secret_templateid = 6008
password_wo_version   = 1
fields = [
  {
    fieldname = "Server"
    itemvalue = ""
  },
  {
    fieldname = "Port"
    itemvalue = ""
  },
  {
    fieldname = "Database"
    itemvalue = ""
  },
  {
    fieldname = "Username"
    itemvalue = ""
  },
  {
    # Password fields are write-only: supply password_value, never itemvalue.
    fieldname      = "Password"
    is_password    = true
    password_value = "Passw0rd."
  },
  {
    fieldname = "Notes"
    itemvalue = ""
  }
]
