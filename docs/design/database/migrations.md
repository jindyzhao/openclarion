# Database Migrations

Atlas migrations are the canonical database migration artifacts.

Rules:

1. Ent schema changes must generate reviewed migrations.
2. destructive migrations require a migration note and rollback plan.
3. generated Ent code must be committed with schema changes.
4. tests must use PostgreSQL, not SQLite, once integration tests exist.
