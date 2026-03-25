#!/bin/bash
# Initialize PostgreSQL database for Agent Infrastructure

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-admin}"
DB_PASSWORD="${DB_PASSWORD:-changeme}"
DB_NAME="${DB_NAME:-copcon}"

echo "Creating database $DB_NAME..."

# Create database
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d postgres -c "CREATE DATABASE $DB_NAME;" 2>/dev/null || echo "Database already exists or using existing"

# Run schema
echo "Creating tables..."
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f /data/copcon/server/internal/session/schema.sql

echo "Done!"