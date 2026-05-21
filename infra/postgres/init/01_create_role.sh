#!/bin/bash
# 01_create_roles.sh
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE ROLE app_system LOGIN PASSWORD '$APP_SYSTEM_PASSWORD' BYPASSRLS;
    CREATE ROLE app_user LOGIN PASSWORD '$APP_USER_PASSWORD';
    GRANT ALL ON SCHEMA public TO app_system;
    GRANT app_user TO app_system WITH ADMIN OPTION;
EOSQL
