-- Drop index "alertevent_source_canonical_fingerprint_starts_at" from table: "alert_events"
DROP INDEX "alertevent_source_canonical_fingerprint_starts_at";
-- Backfill legacy unbound events before enforcing the profile-scoped key.
UPDATE "alert_events" SET "alert_source_profile_id" = 0 WHERE "alert_source_profile_id" IS NULL;
-- Modify "alert_events" table
ALTER TABLE "alert_events" ALTER COLUMN "alert_source_profile_id" SET NOT NULL, ALTER COLUMN "alert_source_profile_id" SET DEFAULT 0;
-- Create index "alertevent_alert_source_profile_id_source_canonical_fingerprint" to table: "alert_events"
CREATE UNIQUE INDEX "alertevent_alert_source_profile_id_source_canonical_fingerprint" ON "alert_events" ("alert_source_profile_id", "source", "canonical_fingerprint", "starts_at");
