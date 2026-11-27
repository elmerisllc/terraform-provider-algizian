# Changelog

All notable changes to this provider. The format follows [Keep a Changelog](https://keepachangelog.com/), versions follow [Semantic Versioning](https://semver.org/).

## Unreleased

- Documented installing a release without the Registry (download, verify the signature, local plugin folder) and added the public signing key as `KEYS.asc`.

## 0.1.0 - 2026-11-27

First release:

- Resources: `algizian_tenant`, `algizian_custom_domain`, `algizian_client`, `algizian_provider_credentials`, `algizian_email_settings`, `algizian_service_account`, `algizian_service_account_token`.
- Data sources: `algizian_tenant`, `algizian_tenants` (filter by tags), `algizian_providers`.
- Tags on tenants, clients, service accounts and their tokens.
- `locked` on tenants: the API refuses to delete a locked tenant.
- Authentication with an organization token; the organization is read from the token.
