resource "algizian_email_settings" "myapp" {
  tenant       = algizian_tenant.myapp.slug
  sender       = "own"
  host         = "smtp.example.com"
  port         = 587
  security     = "starttls"
  username     = "mailer"
  password     = var.smtp_password
  from_address = "login@example.com"
  from_name    = "MyApp"
  template     = "graphic"
}
