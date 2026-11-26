package provider_test

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/elmerisllc/terraform-provider-algizian/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// These tests run the provider through the real Terraform CLI against the in-memory fake (fake_test.go).
// They need `terraform` in PATH and TF_ACC=1.

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"algizian": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func init() { provider.SetDomainPoll(10 * time.Millisecond) }

func cfg(f *fakeAPI, body string) string {
	return fmt.Sprintf(`
provider "algizian" {
  endpoint = "%s/api/v1"
  token    = "ag_test"
  org      = "acme"
}
%s`, f.srv.URL, body)
}

func TestTenantLifecycle(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		CheckDestroy: func(*terraform.State) error {
			if len(f.tenants) != 0 {
				return fmt.Errorf("tenants left: %d", len(f.tenants))
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: cfg(f, `resource "algizian_tenant" "t" {
  name = "Acme"
  branding = { display_name = "Acme Games", primary_color = "#112233", sign_in_title = "Welcome" }
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("algizian_tenant.t", "slug", regexp.MustCompile(`^t\d{6}$`)),
					resource.TestCheckResourceAttr("algizian_tenant.t", "org", "acme"),
					resource.TestCheckResourceAttr("algizian_tenant.t", "name", "Acme"),
					resource.TestCheckResourceAttr("algizian_tenant.t", "providers.#", "4"),
					resource.TestCheckResourceAttr("algizian_tenant.t", "branding.sign_in_title", "Welcome"),
					resource.TestMatchResourceAttr("algizian_tenant.t", "issuer", regexp.MustCompile(`^https://t\d{6}\.algizian\.test$`)),
				),
			},
			{ // a removed branding attribute is cleared with null, a changed one is patched, the providers shrink
				Config: cfg(f, `resource "algizian_tenant" "t" {
  name = "Acme"
  providers = ["google", "email"]
  branding = { display_name = "Acme Games", primary_color = "#445566" }
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("algizian_tenant.t", "branding.primary_color", "#445566"),
					resource.TestCheckNoResourceAttr("algizian_tenant.t", "branding.sign_in_title"),
					resource.TestCheckResourceAttr("algizian_tenant.t", "providers.#", "2"),
					func(*terraform.State) error {
						for _, tn := range f.tenants {
							if _, has := tn["branding"].(map[string]any)["signInTitle"]; has {
								return fmt.Errorf("signInTitle was not cleared on the server")
							}
						}
						return nil
					},
				),
			},
			{ResourceName: "algizian_tenant.t", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestOrgTenantRefusesASlug(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      cfg(f, `resource "algizian_tenant" "t" { slug = "myapp-login" }`),
			ExpectError: regexp.MustCompile(`name is assigned`),
		}},
	})
}

func TestPlatformTenantWithSlug(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
provider "algizian" {
  endpoint = "%s/api/v1"
  token    = "ag_platform"
}
resource "algizian_tenant" "t" { slug = "acme-one" }`, f.srv.URL),
			Check: resource.TestCheckResourceAttr("algizian_tenant.t", "id", "acme-one"),
		}},
	})
}

func TestCustomDomainWaitsForActive(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg(f, `
resource "algizian_tenant" "t" {}
resource "algizian_custom_domain" "d" {
  tenant          = algizian_tenant.t.slug
  hostname        = "login.example.com"
  wait_for_active = true
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("algizian_custom_domain.d", "status", "active"),
					resource.TestCheckResourceAttr("algizian_custom_domain.d", "cname_target", "cname.algizian.test"),
					resource.TestCheckResourceAttr("algizian_custom_domain.d", "cname_name", "login.example.com"),
					resource.TestCheckResourceAttr("algizian_custom_domain.d", "issuer", "https://login.example.com"),
				),
			},
			{ // changing the hostname is an in-place update
				Config: cfg(f, `
resource "algizian_tenant" "t" {}
resource "algizian_custom_domain" "d" {
  tenant          = algizian_tenant.t.slug
  hostname        = "id.example.com"
  wait_for_active = true
}`),
				Check: resource.TestCheckResourceAttr("algizian_custom_domain.d", "issuer", "https://id.example.com"),
			},
		},
	})
}

func TestClientSecretIsKeptAndPublicHasNone(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg(f, `
resource "algizian_tenant" "t" {}
resource "algizian_client" "web" {
  tenant        = algizian_tenant.t.slug
  name          = "Web"
  type          = "confidential"
  redirect_uris = ["https://app.example.com/cb"]
}
resource "algizian_client" "mobile" {
  tenant        = algizian_tenant.t.slug
  name          = "Mobile"
  redirect_uris = ["com.example.app:/cb"]
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("algizian_client.web", "client_secret", regexp.MustCompile(`^cs_c_`)),
					resource.TestCheckNoResourceAttr("algizian_client.mobile", "client_secret"),
					resource.TestCheckResourceAttr("algizian_client.mobile", "type", "public"),
					resource.TestCheckResourceAttr("algizian_client.mobile", "first_party", "true"),
					resource.TestCheckResourceAttr("algizian_client.mobile", "grant_types.#", "2"),
				),
			},
			{ // an update keeps the secret
				Config: cfg(f, `
resource "algizian_tenant" "t" {}
resource "algizian_client" "web" {
  tenant        = algizian_tenant.t.slug
  name          = "Web app"
  type          = "confidential"
  redirect_uris = ["https://app.example.com/cb", "https://app.example.com/cb2"]
}
resource "algizian_client" "mobile" {
  tenant        = algizian_tenant.t.slug
  name          = "Mobile"
  redirect_uris = ["com.example.app:/cb"]
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("algizian_client.web", "name", "Web app"),
					resource.TestCheckResourceAttr("algizian_client.web", "redirect_uris.#", "2"),
					resource.TestMatchResourceAttr("algizian_client.web", "client_secret", regexp.MustCompile(`^cs_c_`)),
				),
			},
		},
	})
}

func TestProviderCredentials(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg(f, `
resource "algizian_tenant" "t" {}
resource "algizian_provider_credentials" "g" {
  tenant        = algizian_tenant.t.slug
  provider_name = "google"
  client_id     = "123-abc.apps.googleusercontent.com"
  client_secret = "s3cret"
}`),
				Check: resource.TestCheckResourceAttr("algizian_provider_credentials.g", "client_id", "123-abc.apps.googleusercontent.com"),
			},
		},
	})
}

func TestEmailSettings(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg(f, `
resource "algizian_tenant" "t" {}
resource "algizian_email_settings" "e" {
  tenant    = algizian_tenant.t.slug
  sender    = "platform"
  template  = "graphic"
  daily_cap = 50
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("algizian_email_settings.e", "from_address", "no-reply@algizian.test"),
					resource.TestCheckResourceAttr("algizian_email_settings.e", "ready", "true"),
					resource.TestCheckResourceAttr("algizian_email_settings.e", "daily_cap", "50"),
				),
			},
			{
				Config: cfg(f, `
resource "algizian_tenant" "t" {}
resource "algizian_email_settings" "e" {
  tenant       = algizian_tenant.t.slug
  sender       = "own"
  host         = "smtp.example.com"
  port         = 587
  security     = "starttls"
  username     = "mailer"
  password     = "pw"
  from_address = "login@example.com"
}`),
				Check: resource.TestCheckResourceAttr("algizian_email_settings.e", "host", "smtp.example.com"),
			},
		},
	})
}

func TestServiceAccountAndToken(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: cfg(f, `
resource "algizian_service_account" "ci" {
  name = "ci"
  role = "admin"
}
resource "algizian_service_account_token" "ci" {
  service_account_id = algizian_service_account.ci.id
  name               = "deploy"
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("algizian_service_account.ci", "org", "acme"),
					resource.TestMatchResourceAttr("algizian_service_account_token.ci", "token", regexp.MustCompile(`^ag_secret_`)),
				),
			},
			{ // a role change is an in-place update; the token keeps working
				Config: cfg(f, `
resource "algizian_service_account" "ci" {
  name = "ci"
  role = "read"
}
resource "algizian_service_account_token" "ci" {
  service_account_id = algizian_service_account.ci.id
  name               = "deploy"
}`),
				Check: resource.TestCheckResourceAttr("algizian_service_account.ci", "role", "read"),
			},
			{ResourceName: "algizian_service_account.ci", ImportState: true, ImportStateIdFunc: func(s *terraform.State) (string, error) {
				return "acme/" + s.RootModule().Resources["algizian_service_account.ci"].Primary.ID, nil
			}, ImportStateVerify: true},
		},
	})
}

func TestDataSources(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: cfg(f, `
resource "algizian_tenant" "t" {}
data "algizian_tenant" "t" { slug = algizian_tenant.t.slug }
data "algizian_providers" "all" {}`),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestMatchResourceAttr("data.algizian_tenant.t", "issuer", regexp.MustCompile(`^https://t\d{6}\.algizian\.test$`)),
				resource.TestCheckResourceAttr("data.algizian_providers.all", "providers.0.provider_name", "google"),
			),
		}},
	})
}

func TestMissingTokenIsReported(t *testing.T) {
	t.Setenv("ALGIZIAN_TOKEN", "")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: `provider "algizian" {}
data "algizian_providers" "all" {}`,
			ExpectError: regexp.MustCompile(`Missing API token`),
		}},
	})
}

func TestAppleCredentialsNeedKeyMaterial(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: cfg(f, `resource "algizian_provider_credentials" "a" {
  tenant        = "acme"
  provider_name = "apple"
  client_id     = "com.example.signin"
  team_id       = "TEAM123456"
}`),
			ExpectError: regexp.MustCompile(`(?s)key_id.*is required`),
		}},
	})
}

func TestEmailPortMustBeATlsPort(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: cfg(f, `resource "algizian_email_settings" "e" {
  tenant = "acme"
  sender = "own"
  port   = 25
}`),
			ExpectError: regexp.MustCompile(`465`),
		}},
	})
}

// A token that belongs to an organization is enough: the org comes from GET /me. Tags are set, replaced, cleared and filtered.
func TestTagsAndTenantListWithOrgFromToken(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	noOrg := func(body string) string {
		return fmt.Sprintf("provider \"algizian\" {\n  endpoint = \"%s/api/v1\"\n  token    = \"ag_test\"\n}\n%s", f.srv.URL, body)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: noOrg(`
resource "algizian_tenant" "myapp" {
  name = "MyApp"
  tags = { product = "myapp", env = "prod" }
}
resource "algizian_tenant" "other" {
  name = "Other"
  tags = { product = "other" }
}
resource "algizian_client" "web" {
  tenant        = algizian_tenant.myapp.slug
  name          = "Web"
  redirect_uris = ["https://app.example.com/cb"]
  tags          = { app = "web" }
}
resource "algizian_service_account" "ci" {
  name = "ci"
  role = "read"
  tags = { system = "ci" }
}
resource "algizian_service_account_token" "ci" {
  service_account_id = algizian_service_account.ci.id
  name               = "k"
  tags               = { purpose = "deploy" }
}
data "algizian_tenants" "myapp" {
  tags       = { product = "myapp" }
  depends_on = [algizian_tenant.myapp, algizian_tenant.other]
}
data "algizian_tenants" "all" {
  depends_on = [algizian_tenant.myapp, algizian_tenant.other]
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("algizian_tenant.myapp", "org", "acme"),
					resource.TestCheckResourceAttr("algizian_tenant.myapp", "tags.product", "myapp"),
					resource.TestCheckResourceAttr("algizian_client.web", "tags.app", "web"),
					resource.TestCheckResourceAttr("algizian_service_account.ci", "tags.system", "ci"),
					resource.TestCheckResourceAttr("algizian_service_account_token.ci", "tags.purpose", "deploy"),
					resource.TestCheckResourceAttr("data.algizian_tenants.myapp", "tenants.#", "1"),
					resource.TestCheckResourceAttr("data.algizian_tenants.myapp", "tenants.0.name", "MyApp"),
					resource.TestCheckResourceAttr("data.algizian_tenants.all", "tenants.#", "2"),
				),
			},
			{ // replace, then clear
				Config: noOrg(`
resource "algizian_tenant" "myapp" {
  name = "MyApp"
  tags = { env = "staging" }
}
resource "algizian_tenant" "other" {
  name = "Other"
  tags = {}
}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("algizian_tenant.myapp", "tags.%", "1"),
					resource.TestCheckResourceAttr("algizian_tenant.myapp", "tags.env", "staging"),
					resource.TestCheckResourceAttr("algizian_tenant.other", "tags.%", "0"),
				),
			},
		},
	})
}

func TestRefusedTokenIsReported(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      fmt.Sprintf("provider \"algizian\" {\n  endpoint = \"%s/api/v1\"\n  token    = \"wrong\"\n}\ndata \"algizian_providers\" \"all\" {}", f.srv.URL),
			ExpectError: regexp.MustCompile(`token was refused`),
		}},
	})
}

func TestTenantLock(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	locked := func(v string) string {
		return cfg(f, fmt.Sprintf(`resource "algizian_tenant" "t" {
  locked = %s
}`, v))
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: locked("true"), Check: resource.TestCheckResourceAttr("algizian_tenant.t", "locked", "true")},
			{Config: locked("true"), Destroy: true, ExpectError: regexp.MustCompile(`(?s)tenant is locked.*locked = false`)},
			{Config: locked("false"), Check: resource.TestCheckResourceAttr("algizian_tenant.t", "locked", "false")},
		},
	})
}

func TestTenantIsUnlockedByDefault(t *testing.T) {
	f := newFake()
	defer f.srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: cfg(f, `resource "algizian_tenant" "t" {}`), Check: resource.TestCheckResourceAttr("algizian_tenant.t", "locked", "false")},
		},
	})
}
