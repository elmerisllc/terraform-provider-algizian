resource "algizian_client" "web" {
  tenant        = algizian_tenant.myapp.slug
  name          = "MyApp web"
  type          = "confidential"
  redirect_uris = ["https://app.example.com/auth/callback"]
  tags          = { app = "web" }
}

resource "algizian_client" "mobile" {
  tenant        = algizian_tenant.myapp.slug
  name          = "MyApp mobile"
  redirect_uris = ["com.example.myapp:/oauth2redirect"]
}

output "web_client_id" {
  value = algizian_client.web.id
}

output "web_client_secret" {
  value     = algizian_client.web.client_secret
  sensitive = true
}
