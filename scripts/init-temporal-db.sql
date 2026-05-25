-- Create Temporal database user and databases.
-- This script runs on first PostgreSQL initialization only.

CREATE USER temporal WITH PASSWORD 'temporal_dev';

CREATE DATABASE temporal OWNER temporal;
CREATE DATABASE temporal_visibility OWNER temporal;
