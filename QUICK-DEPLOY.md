# Quick Deploy Reference

## Prerequisites

- Docker and Docker Compose installed

## Setup

```bash
# 1. Create deployment directory
mkdir -p ~/infinit-feeding && cd ~/infinit-feeding

# 2. Download files
curl -O https://raw.githubusercontent.com/howard3/infinit-feeding/master/docker-compose.yml
curl -O https://raw.githubusercontent.com/howard3/infinit-feeding/master/.sample.env
cp .sample.env .env

# 3. Edit configuration
nano .env  # Fill in your values

# 4. Start the application
docker-compose up -d
```

## Configuration

Edit `.env` with your values:

```bash
# S3 Storage
S3_ENDPOINT=https://...
S3_ACCESS_KEY=...
S3_SECRET_KEY=...
S3_BUCKET_NAME=...
S3_REGION=...

# Turso Database (auth token embedded in URL)
DB_URI=libsql://[DATABASE].turso.io?authToken=[TOKEN]

# Clerk Auth
CLERK_SECRET_KEY=sk_...
CLERK_PUBLISHABLE_KEY=pk_...

# API Key for external consumers
API_KEY=...

# Environment
GO_ENV=production
```

**Port**: 3000  
**Health Check**: `curl http://localhost:3000/health`

