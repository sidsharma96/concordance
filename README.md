# Concordance

Concordance is a Go project for searching technical documentation by product
version and citing the exact source revision behind a result. Code search is a
possible later extension.

The command-line runtime currently provides help and version commands. Importing,
indexing and searching are not available yet; internal capture and storage components
are under development.

## Getting started

Install Go 1.27 or later, Git and Make, then run from this checkout:

```sh
make build
./bin/concordance --help
./bin/concordance version
```

The version command prints `concordance dev`. The scaffold requires no database,
Cloudflare credentials or external service.

See [CONTRIBUTING.md](CONTRIBUTING.md) for development checks and
[the architecture direction](engineering/ARCHITECTURE.md) for the proposed system.
