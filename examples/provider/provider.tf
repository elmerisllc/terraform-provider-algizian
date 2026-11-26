terraform {
  required_providers {
    algizian = {
      source = "elmerisllc/algizian"
    }
  }
}

# The token comes from ALGIZIAN_TOKEN. An org admin token or a service-account
# credential names its organization, so `org` is only needed with a platform token.
provider "algizian" {}
