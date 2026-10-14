-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Database and User Creation (run as postgres superuser)
--
-- Roles are created without passwords: no credential lives in this file.
-- db/init.sh sets each one from its environment variable (APP_KANBAN_PASSWORD,
-- ...); a role whose variable is unset cannot log in with a password.
CREATE DATABASE unheaded_app;
CREATE DATABASE unheaded_ops;
CREATE DATABASE unheaded_config;
CREATE USER app_kanban;
CREATE USER app_timeguru;
CREATE USER app_zhen;
CREATE USER ops_writer;
CREATE USER ops_reader;
CREATE USER config_admin;
CREATE USER config_reader;
