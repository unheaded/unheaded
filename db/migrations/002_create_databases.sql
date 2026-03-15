-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Database and User Creation (run as postgres superuser)
CREATE DATABASE unheaded_app;
CREATE DATABASE unheaded_ops;
CREATE DATABASE unheaded_config;
CREATE USER app_kanban WITH PASSWORD 'kanban_dev';
CREATE USER app_timeguru WITH PASSWORD 'timeguru_dev';
CREATE USER app_zhen WITH PASSWORD 'zhen_dev';
CREATE USER ops_writer WITH PASSWORD 'ops_writer_dev';
CREATE USER ops_reader WITH PASSWORD 'ops_reader_dev';
CREATE USER config_admin WITH PASSWORD 'config_admin_dev';
CREATE USER config_reader WITH PASSWORD 'config_reader_dev';
