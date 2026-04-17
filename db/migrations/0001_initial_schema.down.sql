-- Migration: 0001_initial_schema (rollback)
-- Drops all objects created in 0001_initial_schema.up.sql in reverse
-- dependency order so that foreign-key constraints are satisfied.

DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS migrations;
DROP TABLE IF EXISTS scaling_actions;
DROP TABLE IF EXISTS placements;
DROP TABLE IF EXISTS instances;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS clusters;

DROP FUNCTION IF EXISTS trigger_set_updated_at();
