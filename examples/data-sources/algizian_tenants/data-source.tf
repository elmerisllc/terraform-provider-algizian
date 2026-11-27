# every production tenant of the organization of the token
data "algizian_tenants" "prod" {
  tags = { env = "prod" }
}

output "prod_issuers" {
  value = [for t in data.algizian_tenants.prod.tenants : coalesce(t.custom_domain_issuer, t.issuer)]
}
