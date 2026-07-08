-- +goose Up
CREATE TABLE public.agent_channels (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.agent_channels
    ADD CONSTRAINT agent_channels_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_agent_channels_agent_channel ON public.agent_channels USING btree (agent_id, channel_id);

CREATE INDEX idx_agent_channels_channel_id ON public.agent_channels USING btree (channel_id);

CREATE INDEX idx_agent_channels_org_agent ON public.agent_channels USING btree (org_id, agent_id);

ALTER TABLE ONLY public.agent_channels
    ADD CONSTRAINT fk_agent_channels_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_channels
    ADD CONSTRAINT fk_agent_channels_channel FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_channels
    ADD CONSTRAINT fk_agent_channels_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.agent_channels CASCADE;
