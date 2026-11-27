data "algizian_tenant" "myapp" {
  slug = "k3x9q2a"
}

output "issuer" {
  value = coalesce(data.algizian_tenant.myapp.custom_domain_issuer, data.algizian_tenant.myapp.issuer)
}
