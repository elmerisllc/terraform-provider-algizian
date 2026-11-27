resource "algizian_service_account" "ci" {
  name = "ci"
  role = "admin"
  tags = { system = "github-actions" }
}
