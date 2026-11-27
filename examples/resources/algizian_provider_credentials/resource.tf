resource "algizian_provider_credentials" "google" {
  tenant        = algizian_tenant.myapp.slug
  provider_name = "google"
  client_id     = "123456-example.apps.googleusercontent.com"
  client_secret = var.google_client_secret
}

resource "algizian_provider_credentials" "apple" {
  tenant        = algizian_tenant.myapp.slug
  provider_name = "apple"
  client_id     = "com.example.myapp.signin"
  team_id       = "ABCDE12345"
  key_id        = "KEY1234567"
  private_key   = file("${path.module}/AuthKey_KEY1234567.p8")
}
