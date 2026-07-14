-- Enable the fixed-dimension semantic report index in the application DB.
-- This script runs only when Docker initializes a new local volume; committed
-- Atlas migrations also create the extension for existing installations.

CREATE EXTENSION IF NOT EXISTS vector;
