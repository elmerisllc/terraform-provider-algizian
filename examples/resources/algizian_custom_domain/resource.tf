resource "algizian_custom_domain" "myapp" {
  tenant   = algizian_tenant.myapp.slug
  hostname = "login.example.com"
}

# The DNS record comes from the same configuration (Cloudflare shown).
resource "cloudflare_dns_record" "login" {
  zone_id = var.zone_id
  name    = algizian_custom_domain.myapp.cname_name
  type    = "CNAME"
  content = algizian_custom_domain.myapp.cname_target
  ttl     = 1
  proxied = false
}
