-- +goose Up
-- Reserved migration number.
--
-- The artifacts tables planned for this slot were folded into the initial
-- schema baseline before this migration chain shipped. Keep this no-op so
-- existing production migration version numbers remain stable.

SELECT 1;
