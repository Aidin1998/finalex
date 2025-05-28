#!/bin/sh

app=$1
shift

GOOSE_DRIVER=postgres
GOOSE_DBSTRING=DB_DSN
GOOSE_MIGRATION_DIR="./migrations/$app"
GOOSE_TABLE=goose_migrations

goose "$@"
