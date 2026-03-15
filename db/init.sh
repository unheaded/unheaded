#!/bin/bash
set -e
echo "=== The Well — Initializing databases ==="
psql -v ON_ERROR_STOP=1 -U postgres -f /docker-entrypoint-initdb.d/002_create_databases.sql
psql -v ON_ERROR_STOP=1 -U postgres -d unheaded_app -f /docker-entrypoint-initdb.d/003_app_schema.sql
psql -v ON_ERROR_STOP=1 -U postgres -d unheaded_ops -f /docker-entrypoint-initdb.d/004_ops_schema.sql
psql -v ON_ERROR_STOP=1 -U postgres -d unheaded_config -f /docker-entrypoint-initdb.d/005_config_schema.sql
psql -v ON_ERROR_STOP=1 -U postgres -d unheaded_config -f /docker-entrypoint-initdb.d/006_seed_data.sql
for db in unheaded_app unheaded_ops unheaded_config; do
  psql -v ON_ERROR_STOP=1 -U postgres -d "$db" -f /docker-entrypoint-initdb.d/007_grants.sql
done
echo "=== The Well — Ready ==="
