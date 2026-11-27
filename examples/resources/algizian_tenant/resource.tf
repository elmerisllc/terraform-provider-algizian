resource "algizian_tenant" "myapp" {
  name      = "MyApp"
  providers = ["google", "microsoft", "apple"]

  # Production tenants are locked: neither the API nor `terraform destroy` deletes them.
  # Set this to false and apply first to retire one. Temporary test tenants leave it off.
  locked = true

  branding = {
    display_name  = "MyApp"
    primary_color = "#1f6feb"
    sign_in_title = "Sign in to MyApp"
  }

  tags = {
    product = "myapp"
    env     = "prod"
  }
}

# A temporary tenant for a test run: no lock, so `terraform destroy` removes it.
resource "algizian_tenant" "scratch" {
  name = "Scratch test"
  tags = { purpose = "ci" }
}
