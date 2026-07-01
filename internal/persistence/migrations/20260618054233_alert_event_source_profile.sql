-- Modify "alert_events" table
ALTER TABLE "alert_events" ADD COLUMN "alert_source_profile_id" bigint NULL;
-- Create index "alertevent_alert_source_profile_id_starts_at" to table: "alert_events"
CREATE INDEX "alertevent_alert_source_profile_id_starts_at" ON "alert_events" ("alert_source_profile_id", "starts_at");
