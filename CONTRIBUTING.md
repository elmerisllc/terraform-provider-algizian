# Contributing

Thanks for helping. Bug reports, documentation fixes and pull requests are all welcome.

## Before you start

- For anything bigger than a small fix, open an issue first so we can agree on the approach.
- By contributing you agree that your contribution is licensed under the [MPL-2.0](LICENSE).
- Follow the [code of conduct](CODE_OF_CONDUCT.md).

## Set up

You need the Go version in `go.mod` and Terraform in your `PATH`.

```
make test      # unit tests and the Terraform CLI against an in-memory fake API
make docs      # regenerate docs/ (tfplugindocs); commit the result
make install   # install the provider locally, then use dev_overrides in ~/.terraformrc
```

The tests never call the real service: `internal/provider/fake_test.go` is an in-memory fake of the Algizian management API.

## Pull requests

- One change per pull request, with a test for new behaviour (a Terraform CLI test in `internal/provider/provider_test.go` for resource behaviour).
- Run `gofmt`, `go vet ./...` and `make test`; run `make docs` when you change a schema or description. CI fails when `docs/` is out of date.
- Add examples under `examples/` for a new resource or attribute (they are included in the generated docs).
- Add a line to the "Unreleased" section of [CHANGELOG.md](CHANGELOG.md).
- Secrets (tokens, client secrets) never go in code, tests, examples or issues.

## Conventions

- Attributes use `snake_case`; secrets are `Sensitive`.
- A change that makes a plan differ for existing configurations needs a note in the changelog and, if unavoidable, a major version.
- Do not use an attribute named `provider` (reserved by Terraform); use `provider_name`.

## Releases

Maintainers tag `vX.Y.Z`; the release workflow builds and signs the artifacts for the Terraform Registry.
