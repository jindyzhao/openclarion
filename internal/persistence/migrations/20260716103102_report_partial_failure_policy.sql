-- Modify "final_reports" table
ALTER TABLE "final_reports" ADD COLUMN "generation_status" character varying NOT NULL DEFAULT 'complete', ADD COLUMN "expected_sub_report_count" bigint NOT NULL DEFAULT 1, ADD COLUMN "successful_sub_report_count" bigint NOT NULL DEFAULT 1, ADD COLUMN "failed_sub_report_count" bigint NOT NULL DEFAULT 0;
-- Backfill complete historical reports from their immutable fan-in edges.
UPDATE "final_reports" AS "fr"
SET "expected_sub_report_count" = "links"."sub_report_count",
    "successful_sub_report_count" = "links"."sub_report_count"
FROM (
  SELECT "final_report_id", COUNT(*) AS "sub_report_count"
  FROM "final_report_sub_reports"
  GROUP BY "final_report_id"
) AS "links"
WHERE "fr"."id" = "links"."final_report_id";
-- Modify "report_workflow_policies" table
ALTER TABLE "report_workflow_policies" ADD COLUMN "max_failed_sub_reports" bigint NOT NULL DEFAULT 0;
