# Wealth Tracker

<div align="center">

![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat&logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-3-003B57?style=flat&logo=sqlite&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green?style=flat)

[![Go](https://github.com/AngelFreak/wealth_tracker/actions/workflows/go.yml/badge.svg)](https://github.com/AngelFreak/wealth_tracker/actions/workflows/go.yml)

**A modern, self-hosted personal finance tracker for managing your wealth, investments, and financial goals.**

[Features](#-features) • [Quick Start](#-quick-start) • [Broker Integration](#-broker-integration) • [Development](#-development)

</div>

---

## ✨ Features

### 📊 Dashboard & Analytics
- **Net Worth Overview** - Real-time visualization of your total wealth
- **Interactive Charts** - Track trends over time with beautiful graphs
- **KPI Cards** - Quick insights into your financial health

### 💰 Account Management
- **Assets & Liabilities** - Track everything from stocks to mortgages
- **Categories** - Organize accounts by type (investments, cash, property, crypto, etc.)
- **Multi-Currency** - Support for multiple currencies with live exchange rates
- **Transaction History** - Record income, expenses, and transfers

### 🎯 Financial Goals
- **Goal Tracking** - Set targets and monitor progress
- **Category-Based Goals** - Link goals to specific account categories
- **Visual Progress** - See how close you are to financial independence

### 🔗 Broker Integration
- **Nordnet** - Danish/Nordic broker with MitID authentication
- **Saxo Bank** - OAuth-based integration for Saxo accounts
- **Auto-Sync** - Automatically fetch positions and balances
- **Holdings View** - See all your investments in one place

### 🧮 Financial Calculators
- **FIRE Calculator** - Plan your path to Financial Independence, Retire Early
- **Compound Interest** - Visualize the power of compound growth
- **Danish Salary Calculator** - Calculate net salary with Danish tax rules

### 🎨 User Experience
- **Dark/Light Mode** - Follows system preference or manual toggle
- **Responsive Design** - Works on desktop, tablet, and mobile
- **Fast & Modern** - Built with HTMX for snappy interactions

---

## 🚀 Quick Start

### Using Docker (Recommended)

```bash
# Clone the repository
git clone https://github.com/AngelFreak/wealth_tracker.git
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
```

**Default credentials:** `admin@localhost` / `changeme`

> ⚠️ **Important:** Change the default password immediately after first login!

---

## 🔗 Broker Integration

### Nordnet (Denmark/Nordic)

Connect your Nordnet account using MitID authentication:

1. Go to **Settings** → **Connections** → **Add Connection**
2. Select **Nordnet** and enter your CPR number
3. Scan the QR code with your MitID app
4. Map your Nordnet accounts to local accounts

### Saxo Bank

Connect your Saxo Bank account using OAuth:

1. Create an app at [Saxo Developer Portal](https://developer.saxo/)
2. Go to **Settings** → **Connections** → **Add Connection**
3. Select **Saxo** and enter your App Key and Secret
4. Complete the OAuth login flow
5. Map your Saxo accounts to local accounts

> **Note:** Saxo integration requires a registered developer application. See [Saxo OpenAPI docs](https://developer.saxo/) for setup instructions.

---

## 🛠️ Development

### Prerequisites

- Go 1.24+
- Node.js 18+ (for Tailwind CSS)
- [Air](https://github.com/air-verse/air) (for hot reload)
- Docker (optional)

### Local Setup

```bash
# Clone and enter directory
git clone https://github.com/AngelFreak/wealth_tracker.git
cd wealth_tracker

# Install dependencies
go mod download
npm install

# Install Air for hot reload
go install github.com/air-verse/air@latest

# Build CSS
npm run css

# Run the server (with hot reload)
air
```

Access at `http://localhost:8081`

### Development Commands

```bash
# Start development server with hot reload (recommended)
air

# Watch CSS changes (run in separate terminal)
npm run css:watch

# Run tests
go test ./...

# Build production binary
go build -o ./bin/wealth_tracker ./cmd/server
```

### Air Hot Reload

The project includes an `.air.toml` configuration for automatic rebuilding when files change:

- Watches: `.go`, `.html`, `.css`, `.js`, `.json` files
- Excludes: `tmp/`, `vendor/`, `node_modules/`, `.git/`, `data/`
- Auto-rebuilds and restarts the server on changes

> **Note:** When adding new Tailwind CSS classes, run `npm run css` to regenerate the CSS file.

---

## 📁 Project Structure

```
wealth_tracker/
├── cmd/server/          # Application entry point
├── internal/
│   ├── auth/            # Authentication & sessions
│   ├── broker/          # Broker integrations
│   │   ├── nordnet/     # Nordnet + MitID
│   │   └── saxo/        # Saxo Bank OAuth
│   ├── config/          # Configuration
│   ├── database/        # SQLite & migrations
│   ├── handlers/        # HTTP handlers
│   ├── middleware/      # Auth middleware
│   ├── models/          # Data models
│   ├── repository/      # Database layer
│   └── services/        # Business logic
├── web/
│   ├── static/          # CSS, JS, images
│   └── templates/       # Go HTML templates
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

---

## ⚙️ Tech Stack

| Layer | Technology |
|-------|------------|
| **Backend** | Go 1.24, Chi Router |
| **Database** | SQLite (pure Go driver) |
| **Frontend** | HTMX, Alpine.js |
| **Styling** | Tailwind CSS |
| **Icons** | Lucide Icons |
| **Auth** | Session-based, MitID, OAuth2 |
| **Container** | Docker, Docker Compose |

---

## 🔧 Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `8080` |
| `HOST` | Server host | `localhost` |
| `DB_PATH` | SQLite database path | `data/wealth.db` |
| `SESSION_SECRET` | Cookie signing key | *required* |
| `ENCRYPTION_SECRET` | Credential encryption (32 chars) | *required* |
| `ENV` | Environment mode | `development` |
| `TZ` | Timezone | `Europe/Copenhagen` |

---

## 🚢 Deployment

### Docker (Recommended)

```bash
# Start
docker compose up -d

# View logs
docker compose logs -f

# Stop
docker compose down
```

Data persists in `./data` directory.

### With HTTPS (Caddy)

```bash
# Create Caddyfile
echo "wealth.yourdomain.com { reverse_proxy wealth-tracker:8080 }" > Caddyfile

# Uncomment Caddy service in docker-compose.yml and start
docker compose up -d
```

---

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

---

<div align="center">

**Built with ❤️ for personal finance nerds**

</div>
