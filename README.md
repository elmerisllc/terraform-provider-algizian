# Terraform provider for Algizian

[![test](https://github.com/elmerisllc/terraform-provider-algizian/actions/workflows/test.yml/badge.svg)](https://github.com/elmerisllc/terraform-provider-algizian/actions/workflows/test.yml)

Manage [Algizian](https://algizian.com) (OAuth 2.1 / OpenID Connect with social logins) from Terraform: tenants and their branding, custom domains, OIDC clients, your own Google/Microsoft/Apple apps, email sign-in, service accounts and credentials, with free-form **tags** on every object.

```hcl
terraform {
  required_providers {
    algizian = { source = "elmerisllc/algizian" }
  }
}

provider "algizian" {} # the token comes from ALGIZIAN_TOKEN

resource "algizian_tenant" "app" {
  name   = "My app"
  locked = true # production: the API refuses to delete it until this is false
}

resource "algizian_custom_domain" "app" {
  tenant   = algizian_tenant.app.slug
  hostname = "login.example.com"
}

resource "algizian_client" "web" {
  tenant        = algizian_tenant.app.slug
  name          = "Web"
  redirect_uris = ["https://app.example.com/auth/callback"]
}
```

Every resource and data source has a reference page with arguments, attributes, examples and import syntax in [docs/](docs/), and complete examples in [examples/](examples/). [llms.txt](llms.txt) lists the same pages for AI assistants.

## Install

With the Terraform Registry, the `required_providers` block above is all you need. To install without the Registry, download the release files from [GitHub Releases](https://github.com/elmerisllc/terraform-provider-algizian/releases). Each release carries a zip per platform, a `SHA256SUMS` file and its signature `SHA256SUMS.sig`, signed with the key `ED44 4BC2 F9FA 6B97 77FA  2DD3 2AAD E6A6 6F09 51D4` (public key: [KEYS.asc](KEYS.asc)).

```sh
VERSION=0.1.0; OS=linux; ARCH=amd64   # OS: linux, darwin, windows, freebsd; ARCH: amd64, arm64, 386, arm
BASE=https://github.com/elmerisllc/terraform-provider-algizian/releases/download/v$VERSION
ZIP=terraform-provider-algizian_${VERSION}_${OS}_${ARCH}.zip
SUMS=terraform-provider-algizian_${VERSION}_SHA256SUMS

cd "$(mktemp -d)"
curl -fsSLO "$BASE/$ZIP" -O "$BASE/$SUMS" -O "$BASE/$SUMS.sig"
curl -fsSLO https://raw.githubusercontent.com/elmerisllc/terraform-provider-algizian/main/KEYS.asc

# verify: Terraform does not check the signature of a manually installed provider
gpg --import KEYS.asc
gpg --verify "$SUMS.sig" "$SUMS"                 # expect "Good signature", fingerprint above
sha256sum --ignore-missing -c "$SUMS"            # macOS: shasum -a 256 --ignore-missing -c "$SUMS"

# install into Terraform's default local plugin folder (Windows: %APPDATA%\terraform.d\plugins)
DIR=~/.terraform.d/plugins/registry.terraform.io/elmerisllc/algizian
mkdir -p "$DIR" && mv "$ZIP" "$DIR/"
```


Use `source = "elmerisllc/algizian"` and `version = "0.1.0"` in `required_providers`, and run `terraform init`; Terraform finds the zip there and reports it as unauthenticated. To install on several platforms or machines, put the zip in a shared folder and point a `filesystem_mirror` block in your CLI configuration at it, see [Terraform's provider installation settings](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-installation). Run `terraform providers lock -fs-mirror=<folder> -platform=<os_arch>` once per platform to record every platform's checksum in `.terraform.lock.hcl`.

## Authentication

One API token, from `ALGIZIAN_TOKEN` or the `token` argument. The normal case is a token that belongs to an organization (an org admin token or a service-account credential): the provider asks `GET /me` and uses that organization for tenants and service accounts, so the provider block can be empty. A platform token needs `org` (or `ALGIZIAN_ORG`) to create organization tenants, or a `slug` to create platform tenants.

| What | Org admin token or admin service account | Platform token |
|---|---|---|
| create and configure tenants, branding, providers, email | yes (the name `<slug>.algizian.com` is assigned) | yes (may choose a slug) |
| custom domains, clients, provider apps | yes (within the plan) | yes |
| service accounts and their tokens | org admin token only, not a service account | yes |
| delete a tenant | yes, its own org's tenants, unless locked | yes, unless locked |
| read tenants and their issuers | yes | yes |

## Resources and data sources

Resources: `algizian_tenant`, `algizian_custom_domain`, `algizian_client`, `algizian_provider_credentials`, `algizian_email_settings`, `algizian_service_account`, `algizian_service_account_token`. Data sources: `algizian_tenant`, `algizian_tenants` (filter by tags), `algizian_providers`.

## Things to know

- **The issuer is the custom domain.** Attach `algizian_custom_domain` before any client or token depends on the tenant; the issuer (`iss`) changes when the domain becomes active.
- **Secrets live in the state.** The API shows a client secret and a service-account token once; they are stored in the state (sensitive). Algizian never returns provider secrets or the SMTP password, so they are in the state too. Protect the state.
- **Tags** replace the whole map of an object; `tags = {}` clears them. Tokens take their tags at creation only.
- **Destroying a tenant deletes all its users, keys and data.** Temporary test tenants clean up with `terraform destroy`.
- **Lock production tenants.** Set `locked = true`: the API then refuses to delete the tenant (409) for every token until `locked = false` is applied, so an accidental `terraform destroy` or API call cannot remove it.
- Your own provider apps change the user's `sub` (Microsoft per app, Apple per team): decide before users exist.

## Support and contributing

- Questions and ideas: [GitHub Discussions](https://github.com/elmerisllc/terraform-provider-algizian/discussions) or an issue, see [SUPPORT.md](SUPPORT.md).
- Bugs and feature requests: [issues](https://github.com/elmerisllc/terraform-provider-algizian/issues/new/choose).
- Security problems: [SECURITY.md](SECURITY.md); please do not open a public issue.
- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md). Conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Changes: [CHANGELOG.md](CHANGELOG.md).

## Develop

```
make test      # unit tests + the Terraform CLI against an in-memory fake API (needs terraform in PATH)
make docs      # regenerate docs/ with tfplugindocs
make install   # go install, then use a dev_overrides block in ~/.terraformrc
```

## License

[MPL-2.0](LICENSE).
