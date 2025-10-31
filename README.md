# Branchd

Branchd is a free PostgreSQL database branching tool that you can self-host (currently only on AWS).

- **Great DX** - includes an admin web UI and the branchd CLI for managing branches
- **Instant branches** - Create full database clones in seconds
- **Storage efficient** - Compression and copy-on-write. Perfect for development teams handling large databases
- **Fully isolated** - Each branch runs its own PostgreSQL instance on its own data directory with unique credentials
- **Secure** - Encryption at rest and in transit

## Use Cases

- **Feature Development** - Each developer gets their own database branch
- **Pull Request Environments** - Automated branch creation for every PR
- **QA Testing** - Isolated databases for testing without affecting others
- **Schema Migrations** - Test migrations safely before production
- **Data Experiments** - Try changes without worrying about rollback

## Quick Start

Visit [branchd.dev](https://branchd.dev) for a step-by-step guide.

## Security

The [Cloudformation template](cloudformation/branchd.yaml) and [server setup script](scripts/server_setup.sh) include:

- **TLS/HTTPS** - Self-signed certificates for Postgres connections and https
- **Encryption at rest** - Cloudformation template creates an encrypted EBS volume
- **Firewall** - UFW enabled
- **Intrusion Detection** - fail2ban for PostgreSQL ports
- **Auto-updates** - Unattended security updates (VM restarts on Sundays at 3 AM UTC)

## License

See LICENSE

---

**Built with ❤️ for developers**
