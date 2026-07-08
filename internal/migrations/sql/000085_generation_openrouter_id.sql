-- +goose Up
ALTER TABLE public.generations ADD COLUMN openrouter_generation_id text;
