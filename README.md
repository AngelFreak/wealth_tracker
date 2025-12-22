# Wealth Tracker

A personal wealth tracking application for managing assets, liabilities, transactions, and financial goals. Built with Go, HTMX, Alpine.js, and Tailwind CSS.

## Features

- **Dashboard** - Net worth overview with charts and KPI cards
- **Accounts** - Track assets and liabilities across categories
- **Transactions** - Record income, expenses, and transfers
- **Categories** - Organize accounts by type (stocks, cash, property, etc.)
- **Goals** - Set and track financial targets
- **Multi-currency** - Support for multiple currencies with exchange rates
- **Broker Integration** - Sync with Nordnet (Danish broker)
- **Financial Tools**
  - FIRE Calculator (Financial Independence, Retire Early)
  - Compound Interest Calculator
  - Salary Calculator (Danish tax)
- **Dark/Light Mode** - System preference or manual toggle

## Quick Start with Docker

```bash
# Clone the repository
git clone https://github.com/yourusername/wealth_tracker.git
cd wealth_tracker

# Configure environment
cp .env.example .env

# Generate secure secrets
SESSION_SECRET=$(openssl rand -base64 32)
ENCRYPTION_SECRET=$(openssl rand -base64 24 | head -c 32)
sed -i "s|SESSION_SECRET=.*|SESSION_SECRET=$SESSION_SECRET|" .env
sed -i "s|ENCRYPTION_SECRET=.*|ENCRYPTION_SECRET=$ENCRYPTION_SECRET|" .env

# Start the application
docker compose up -d

# Access at http://localhost:8080
# Default login: admin@localhost / changeme
```

**Important:** Change the default password on first login.

## Development Setup

### Prerequisites

- Go 1.24+
- Node.js 18+ (for Tailwind CSS)

### Installation

```bash
# Clone and enter directory
git clone https://github.com/yourusername/wealth_tracker.git
cd wealth_tracker

# Install Go dependencies
go mod download

# Install Node dependencies
npm install

# Build CSS
npm run css

# Run the server
go run ./cmd/server
```

The application runs at `http://localhost:8080`

### Development Commands

```bash
# Run server with live reload (requires air)
make dev

# Watch and rebuild CSS
npm run css:watch

# Run tests
make test

# Build production binary
make build
```

## Project Structure

```
wealth_tracker/
├── cmd/server/          # Application entry point
├── internal/
│   ├── auth/            # Authentication & sessions
│   ├── broker/          # Broker integrations (Nordnet)
│   ├── config/          # Configuration management
│   ├── database/        # SQLite setup & migrations
│   ├── handlers/        # HTTP request handlers
│   ├── middleware/      # Auth middleware
│   ├── models/          # Data models
│   ├── repository/      # Database access layer
│   ├── services/        # Business logic
│   └── sync/            # Broker synchronization
├── web/
│   ├── static/
│   │   ├── css/         # Tailwind CSS
│   │   └── js/          # Alpine.js components
│   └── templates/       # Go HTML templates
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go with Chi router |
| Database | SQLite (pure Go driver) |
| Frontend | HTMX + Alpine.js |
| Styling | Tailwind CSS |
| Icons | Lucide Icons |
| Containerization | Docker |

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `8080` |
| `HOST` | Server host | `localhost` |
| `DB_PATH` | SQLite database path | `data/wealth.db` |
| `SESSION_SECRET` | Cookie signing key | - |
| `ENCRYPTION_SECRET` | Credential encryption key (32 chars) | - |
| `ENV` | Environment (development/production) | `development` |

## Deployment

### Docker (Recommended)

The application is designed to run as a single Docker container with SQLite for data persistence.

```bash
# Build and run
docker compose up -d

# View logs
docker compose logs -f

# Stop
docker compose down
```

Data is persisted in the `./data` directory.

### With HTTPS (Caddy)

For production deployments, use Caddy as a reverse proxy for automatic HTTPS:

```bash
# Uncomment the Caddy service in docker-compose.yml
# Create Caddyfile with your domain
echo "wealth.yourdomain.com { reverse_proxy wealth-tracker:8080 }" > Caddyfile

# Start services
docker compose up -d
```

## License

MIT License - see [LICENSE](LICENSE) file for details.
