resource "algizian_service_account_token" "ci" {
  service_account_id = algizian_service_account.ci.id
  name               = "deploy"
  expires_at         = "2027-11-26T00:00:00Z"
  tags               = { purpose = "deploy" }
}

output "ci_token" {
  value     = algizian_service_account_token.ci.token
  sensitive = true
}
