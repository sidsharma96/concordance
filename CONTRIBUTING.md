# Contributing

Use Go 1.27 or later, Git and Make.

Git capture tests require support for `git --no-lazy-fetch --version`; upgrade Git
if that command rejects the option. Tests use synthetic local repositories and
never fetch remote objects.

From the checkout, run:

```sh
make check
make build
```

`make check` checks formatting, runs `go vet` and tests, and inspects the Git index
for private or generated artifacts. Stage intended source changes before relying
on the index check. Run `make race` when changing concurrent behavior.

Catalog integration tests use PostgreSQL when `CONCORDANCE_TEST_DATABASE_URL` is set:

```sh
CONCORDANCE_TEST_DATABASE_URL='postgres://localhost/concordance_test' go test ./internal/catalog ./cmd/concordance -count=1
```

Use a dedicated test database with permission to create schemas. Tests create and
remove their own isolated schemas; they do not create or drop the database. Without
the variable, database cases are skipped.

Before opening or updating a pull request, run the full local checks:

```sh
CONCORDANCE_TEST_DATABASE_URL='postgres://localhost/concordance_test' make verify
```

`make verify` requires the database setting, then runs `make check`, an uncached
race-enabled test suite, and `make build`. An unreachable database fails the tests.
GitHub Actions is disabled for this repository; pull requests record the local
results and any checks skipped.
These checks do not establish compatibility with other operating systems or
PostgreSQL versions.

Object-storage tests exercise the real S3 SDK against local HTTP fixtures. The live
R2 probe is opt-in: configure `CONCORDANCE_R2_ENDPOINT`, `CONCORDANCE_R2_BUCKET`,
`R2_ACCESS_KEY_ID` and `R2_SECRET_ACCESS_KEY` locally for a dedicated private test
bucket, then run:

```sh
CONCORDANCE_R2_LIVE_TEST=1 go test ./internal/objectstore -run '^TestR2LiveImmutableObject$' -count=1 -v
```

Use an HTTPS R2 S3 endpoint and bucket-restricted object read/write credentials.
Keep secrets in local credential tooling, never source or shell history. The probe
creates no bucket and leaves one small synthetic object in a unique namespace;
automatic deletion is disabled. Ordinary local checks skip the live probe.

Use `make hooks-install` once per clone to install the versioned pre-commit hook.
It checks staged artifacts and whitespace; it does not replace the normal checks.
The installer refuses to overwrite a different custom hook configuration.

Follow [Go practices](engineering/GO.md). Keep packages focused on concrete
operations, document ownership and failure behavior, and test observable contracts.
Use plain, descriptive branch names such as `posting-intersection`.

Keep pull requests focused. Explain the problem, resulting behavior, relevant
checks and remaining limits. Include a simple baseline and reproducible workload
when making performance claims. Do not commit credentials, generated data, local
review configuration or private book sources. Application builds and ordinary checks
must work without private documentation or external service credentials.
