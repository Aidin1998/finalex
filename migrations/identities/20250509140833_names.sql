-- +goose Up
-- +goose StatementBegin
-- Create "users" table
CREATE TABLE "users" (
  "id" text NOT NULL,
  "provider_id" text NOT NULL,
  "username" text NOT NULL,
  "email" text NOT NULL,
  "first_name" text NULL,
  "last_name" text NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "uni_users_email" to table: "users"
CREATE UNIQUE INDEX "uni_users_email" ON "users" ("email");
-- Create index "uni_users_provider_id" to table: "users"
CREATE UNIQUE INDEX "uni_users_provider_id" ON "users" ("provider_id");
-- Create index "uni_users_username" to table: "users"
CREATE UNIQUE INDEX "uni_users_username" ON "users" ("username");
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE "users";
-- +goose StatementEnd
