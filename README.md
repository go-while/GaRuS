# GaRuS - Github-actions-Runner-upload-Server

**GaRuS** (Github-actions-Runner-upload-Server) is a standalone, minimal, production-grade server for handling secure artifact uploads from GitHub Actions self-hosted runners or other CI/CD pipelines. It supports multi-instance deployments, access control, and token-based authentication with optional network ACLs and TLS.

---

- **Note:** GaRuS is also available as an importable Go module for embedding secure upload endpoints in your own applications.
- See [server/README.md](server/README.md) for usage details.

- **Note:** GaRuS includes a thread-safe, importable token storage module with per-repo, network-restricted, and auto-expiring tokens
- See [tokens/README.md](tokens/README.md) for details.

- **Note:** GaRuS includes a simple, thread-safe network ACL module for IP and subnet-based access control
- See [networkacl/README.md](networkacl/README.md) for details.

---

# Latest Version: pre-v0.0.1

---

## Features

- **Simple Artifact Upload API**: Accepts file uploads via HTTP `POST`.
- **Token-based Authentication**: Secure uploads using per-repository tokens.
- **Network Access Control**: Restrict uploads to specific IPs, subnets, or worldwide.
- **Multi-Instance Support**: Launch multiple isolated servers with different settings.
- **Admin Interface (WIP)**: Add/remove instances and tokens on the fly.
- **Graceful Shutdown**: Handles signals for clean shutdown and resource management.
- **Optional TLS**: Secure uploads over HTTPS via user-supplied certificates.

---

## Usage

### 1. Running the Server

```sh
go run main.go -listen "[::]:58080" -routes "/upload.php" -tokenf "./.passwd" -upload "/tmp/test/garus/uploads"
```

**Flags:**

- `-listen`  : Address to listen on (default: `[::]:58080`)
- `-routes`  : Upload endpoint path (default: `/upload.php`)
- `-tokenf`  : Path to tokens file (default: `./.passwd`)
- `-upload`  : Path to upload directory (default: `/tmp/test/garus/uploads`)
- `-tlscrt`  : Path to TLS certificate (optional)
- `-tlskey`  : Path to TLS private key (optional)

### 2. Uploading Artifacts

Upload with `curl` (or from a GitHub Actions runner):

```sh
curl -F "file=@./artifact.zip" \
     -H "X-Git-Repo: owner/repo" \
     -H "X-Git-Ref: <branch-or-tag>" \
     -H "X-Git-SHA7: <commitsha>" \
     -H "X-Git-Comp: <compiler-info>" \
     -H "X-Auth-Token: <yourtoken>" \
     http://localhost:58080/upload.php
```

**Headers:**

- `X-Git-Repo`   : Repository name (e.g., `owner/repo`)
- `X-Git-Ref`    : Branch or tag name
- `X-Git-SHA7`   : Short commit hash (7 chars)
- `X-Git-Comp`   : Compiler/run info (optional, e.g., `SHR=self-hosted runner`, `GOR=GoReleaser`)
- `X-Auth-Token` : Token for authentication (see below)

### 3. Token & Network Control

Tokens are stored in a "passwd" file, by default `./.passwd`, with each line:

```
repo|token|expires|acl
```

- `repo`: Repository name (`owner/repo`)
- `token`: 20+ char secret token
- `expires`: UNIX timestamp for token expiry
- `acl`: Comma-separated IPs/subnets allowed (e.g., `127.0.0.1,[::1],192.168.0.0/24`)
  - Use `0.0.0.0` for worldwide access.

**Example:**
```
go-while/GaRuS|SOMESECRETKEYTOKEN123456|1999999999|0.0.0.0
```

Tokens are reloaded automatically each minute. Expired tokens are pruned in memory.

---

## Directory Structure

```
.
├── main.go         # CLI entrypoint, admin interface, multi-instance logic
├── networkacl/     # IP/network ACL parsing and checks
│   └── netacl.go
├── server/         # HTTP(S) server, upload handler
│   └── server.go
├── tokens/         # Token file handling, authentication
│   └── tokens.go
└── .passwd         # Example tokens file (see above)
```

---

## Security

- **Tokens**: Minimum 20 chars. Expiry required.
- **Network ACL**: Restrict by IP or subnet for extra safety.
- **TLS**: Use `-tlscrt` and `-tlskey` for production.
- **Admin Interface**: By default not exposed; see `main.go` and TODOs.

---

## TODO / Roadmap

- [ ] Complete Admin Interface for runtime management of instances and tokens
- [ ] REST endpoints for adding/removing tokens live
- [ ] Logging improvements
- [ ] Prometheus metrics/export
- [ ] Docker image

---

## License

MIT. See `LICENSE`.

---

## Credits

- Designed and implemented by [@go-while](https://github.com/go-while).
- Inspired by needs for minimal, CI/CD-friendly artifact upload endpoints.

---

## Example GitHub Actions Step

```yaml
- name: Upload artifact to GaRuS
  run: |
    curl -F "file=@./my-artifact.zip" \
         -H "X-Git-Repo: $GITHUB_REPOSITORY" \
         -H "X-Git-Ref: $GITHUB_REF_NAME" \
         -H "X-Git-SHA7: ${GITHUB_SHA:0:7}" \
         -H "X-Git-Comp: SHR=self-hosted runner" \
         -H "X-Auth-Token: ${{ secrets.GARUS_TOKEN }}" \
         https://your-garus-server/upload.php
```

---

## Support

Open an issue or PR on [GitHub](https://github.com/go-while/GaRuS).