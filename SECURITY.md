# Security policy

## Reporting a vulnerability

Please do not open a public issue for a security problem.

Report it privately through [GitHub private vulnerability reporting](https://github.com/elmerisllc/terraform-provider-algizian/security/advisories/new), or by email to info@elmeris.com with "Security" in the subject.

Include the provider version, what you saw and the steps to reproduce it. We acknowledge a report within 3 business days and keep you informed until it is fixed. We credit reporters in the release notes unless you prefer not to be named.

## What is in scope

This provider only: how it handles the API token, the state and the API answers. Problems in the Algizian service itself (algizian.com) can be reported the same way.

## Handling secrets

The provider writes client secrets, service-account tokens, provider secrets and the SMTP password to the Terraform state as sensitive values. Keep the state in an encrypted backend with restricted access, and never commit state files or tokens.

## Supported versions

Security fixes go into the latest release.
