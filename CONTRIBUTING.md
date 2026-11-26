# Contributing

Bug reports, fixes and small features are welcome.

## Before you start

- **Bugs:** open an issue with the version you run, your configuration with credentials removed, and the relevant logs.
- **Features:** open an issue first, so we can agree on the approach before you write code.
- **Security vulnerabilities:** report them privately, as described in [SECURITY.md](SECURITY.md).

## Development setup

See [Local development](README.md#local-development) in the README.

## Pull requests

- Fork the repository and branch off `main`.
- Keep each pull request to one logical change.
  Split large or complicated changes into stacked pull requests, each small enough to review on its own,
  and link the pull request each one depends on in its description.
- Add or update tests for any change in behavior.
- Update the README when you change configuration, chart values or behavior.
- Leave versions alone. Maintainers set them on release.

## Acceptance criteria

We merge a pull request when:

- All CI checks pass.
- A code owner from [CODEOWNERS](.github/CODEOWNERS) has approved it.
- Review comments are resolved.
- The change fits the scope of the project, as described in the [README](README.md).

## License

By contributing, you agree to license your contributions under the project's [LICENSE](LICENSE).
