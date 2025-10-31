# Branchd

**Database branching for PostgreSQL**

Branchd is a self-hosted database branching service. Each branch is a fully isolated PostgreSQL instance with its own credentials and port, ready in seconds for any database size.

**Key Features:**
- **Instant branches** - Create full database clones in seconds
- **Fully isolated** - Each branch runs its own PostgreSQL instance with unique credentials
- **Storage efficient** - Compression and copy-on-write
- **Multi-version** - Supports PostgreSQL 14, 15, 16, and 17
- **Simple management** - Web UI + CLI for creating, listing, and deleting branches
- **Secure** - Postgres TLS and https by default, fail2ban protection, automatic security updates

## Use Cases

- **Feature Development** - Each developer gets their own database branch
- **Pull Request Environments** - Automated branch creation for every PR
- **QA Testing** - Isolated databases for testing without affecting others
- **Schema Migrations** - Test migrations safely before production
- **Data Experiments** - Try changes without worrying about rollback

## Quick Start

Visit [branchd.dev](https://branchd.dev) for a step-by-step guide.

## Security

Branchd includes security hardening by default:

- **TLS/HTTPS** - Self-signed certificates for Postgres connections and web dashboard https
- **Authentication** - JWT-based auth, bcrypt password hashing
- **Firewall** - UFW enabled
- **Intrusion Detection** - fail2ban for PostgreSQL ports
- **Auto-updates** - Unattended security updates

## License

See LICENSE

---

**Built with ❤️ for developers**
