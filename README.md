# nws

A command line for nws.

`nws` is a single pure-Go binary. It reads public nws data
over plain HTTPS, shapes it into clean records, and prints output that pipes
into the rest of your tools. No API key, nothing to run alongside it.

The same package is also a [resource-URI driver](#use-it-as-a-resource-uri-driver),
so a host program like [ant](https://github.com/tamnd/ant) can address
nws as `nws://` URIs.

## Install

```bash
go install github.com/tamnd/nws-cli/cmd/nws@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/nws-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/nws:latest --help
```

## Usage

```bash
nws page <path>                      # fetch one page as a record
nws page <path> -o json              # as JSON, ready for jq
nws page <path> --template '{{.Body}}'  # just the readable body text
nws links <path>                     # the pages it links to, one per line
nws --help                           # the whole command tree
```

Every command shares one output contract: `-o table|json|jsonl|csv|tsv|url|raw`,
`--fields` to pick columns, `--template` for a custom line, and `-n` to limit.
The default adapts to where output goes (a table on a terminal, JSONL in a
pipe), so the same command reads well by hand and parses cleanly downstream.

This is a fresh scaffold. It ships one example resource type, `page`, wired end
to end. Model the real nws records in `nws/` and declare their
operations in `nws/domain.go`; each one becomes a command, an HTTP
route, and an MCP tool at once.

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents,
with no extra code:

```bash
nws serve --addr :7777    # GET /v1/page/<path>  returns NDJSON
nws mcp                   # speak MCP over stdio
```

## Use it as a resource-URI driver

`nws` registers a `nws` domain the way a program registers a
database driver with `database/sql`. A host enables it with one blank import:

```go
import _ "github.com/tamnd/nws-cli/nws"
```

Then [ant](https://github.com/tamnd/ant) (or any program that links the package)
dereferences `nws://` URIs without knowing anything about nws:

```bash
ant get nws://page/<path>   # fetch the record
ant cat nws://page/<path>   # just the body text
ant ls  nws://page/<path>   # the pages it links to, each addressable
ant url nws://page/<path>   # the live https URL
```

## Development

```
cmd/nws/   thin main: hands cli.NewApp to kit.Run
cli/                 assembles the kit App from the nws domain
nws/                the library: HTTP client, data models, and domain.go (the driver)
docs/                tago documentation site
```

```bash
make build      # ./bin/nws
make test       # go test ./...
make vet        # go vet ./...
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the
archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a
cosign signature:

```bash
git tag v0.1.0
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so the first
release works with no extra secrets.

## License

Apache-2.0. See [LICENSE](LICENSE).
