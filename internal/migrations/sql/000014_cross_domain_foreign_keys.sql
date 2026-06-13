-- +goose Up
-- Cross-domain foreign keys

-- Cross-domain foreign key constraints

ALTER TABLE ONLY api_keys
    ADD CONSTRAINT fk_api_keys_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;


ALTER TABLE ONLY credentials
    ADD CONSTRAINT fk_credentials_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY drive_assets
    ADD CONSTRAINT fk_drive_assets_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY drive_assets
    ADD CONSTRAINT fk_drive_assets_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_assets
    ADD CONSTRAINT fk_agent_assets_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_assets
    ADD CONSTRAINT fk_agent_assets_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_assets
    ADD CONSTRAINT fk_agent_assets_sandbox FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_session_events
    ADD CONSTRAINT fk_agent_session_events_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_session_events
    ADD CONSTRAINT fk_agent_session_events_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_session_events
    ADD CONSTRAINT fk_agent_session_events_sandbox FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_session_events
    ADD CONSTRAINT fk_agent_session_events_session FOREIGN KEY (agent_session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE;


ALTER TABLE ONLY agent_sandbox_upgrades
    ADD CONSTRAINT fk_agent_sandbox_upgrades_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_sandbox_upgrades
    ADD CONSTRAINT fk_agent_sandbox_upgrades_new_sandbox FOREIGN KEY (new_sandbox_id) REFERENCES sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_sandbox_upgrades
    ADD CONSTRAINT fk_agent_sandbox_upgrades_old_sandbox FOREIGN KEY (old_sandbox_id) REFERENCES sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_sandbox_upgrades
    ADD CONSTRAINT fk_agent_sandbox_upgrades_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_sandbox FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_schedule FOREIGN KEY (schedule_id) REFERENCES agent_schedules(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_schedules
    ADD CONSTRAINT fk_agent_schedules_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_schedules
    ADD CONSTRAINT fk_agent_schedules_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_schedules
    ADD CONSTRAINT fk_agent_schedules_sandbox FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_sessions
    ADD CONSTRAINT fk_agent_sessions_credential FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_sessions
    ADD CONSTRAINT fk_agent_sessions_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_sessions
    ADD CONSTRAINT fk_agent_sessions_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_sessions
    ADD CONSTRAINT fk_agent_sessions_sandbox FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_sessions
    ADD CONSTRAINT fk_agent_sessions_token FOREIGN KEY (token_id) REFERENCES tokens(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_skills
    ADD CONSTRAINT fk_agent_skills_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_skills
    ADD CONSTRAINT fk_agent_skills_skill FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_connection FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE SET NULL;

ALTER TABLE ONLY agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_conversation FOREIGN KEY (conversation_id) REFERENCES agent_sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_trigger FOREIGN KEY (trigger_id) REFERENCES agent_triggers(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_triggers
    ADD CONSTRAINT fk_agent_triggers_connection FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_triggers
    ADD CONSTRAINT fk_agent_triggers_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_triggers
    ADD CONSTRAINT fk_agent_triggers_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agents
    ADD CONSTRAINT fk_agents_credential FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE SET NULL;

ALTER TABLE ONLY agents
    ADD CONSTRAINT fk_agents_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY agents
    ADD CONSTRAINT fk_agents_sandbox_template FOREIGN KEY (sandbox_template_id) REFERENCES sandbox_templates(id) ON DELETE SET NULL;

ALTER TABLE ONLY hindsight_banks
    ADD CONSTRAINT fk_hindsight_banks_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY connections
    ADD CONSTRAINT fk_connections_integration FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE CASCADE;

ALTER TABLE ONLY connections
    ADD CONSTRAINT fk_connections_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY connections
    ADD CONSTRAINT fk_connections_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY integrations
    ADD CONSTRAINT fk_integrations_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY integrations
    ADD CONSTRAINT fk_integrations_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY oauth_accounts
    ADD CONSTRAINT fk_oauth_accounts_user FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE ONLY org_invites
    ADD CONSTRAINT fk_org_invites_invited_by FOREIGN KEY (invited_by_id) REFERENCES users(id);

ALTER TABLE ONLY org_invites
    ADD CONSTRAINT fk_org_invites_org FOREIGN KEY (org_id) REFERENCES orgs(id);

ALTER TABLE ONLY org_memberships
    ADD CONSTRAINT fk_org_memberships_org FOREIGN KEY (org_id) REFERENCES orgs(id);

ALTER TABLE ONLY org_memberships
    ADD CONSTRAINT fk_org_memberships_user FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE ONLY rag_index_attempt_errors
    ADD CONSTRAINT fk_rag_index_attempt_errors_index_attempt FOREIGN KEY (index_attempt_id) REFERENCES rag_index_attempts(id) ON DELETE CASCADE;

ALTER TABLE ONLY rag_sources
    ADD CONSTRAINT fk_rag_sources_connection FOREIGN KEY (connection_id) REFERENCES connections(id);

ALTER TABLE ONLY sandbox_templates
    ADD CONSTRAINT fk_sandbox_templates_base_template FOREIGN KEY (base_template_id) REFERENCES sandbox_templates(id);

ALTER TABLE ONLY sandbox_templates
    ADD CONSTRAINT fk_sandbox_templates_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY sandboxes
    ADD CONSTRAINT fk_sandboxes_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY sandboxes
    ADD CONSTRAINT fk_sandboxes_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY sandboxes
    ADD CONSTRAINT fk_sandboxes_sandbox_template FOREIGN KEY (sandbox_template_id) REFERENCES sandbox_templates(id) ON DELETE SET NULL;

ALTER TABLE ONLY skills
    ADD CONSTRAINT fk_skills_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY skills
    ADD CONSTRAINT fk_skills_publisher FOREIGN KEY (publisher_id) REFERENCES users(id) ON DELETE SET NULL;






ALTER TABLE ONLY subscription_change_quotes
    ADD CONSTRAINT fk_subscription_change_quotes_subscription FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE;

ALTER TABLE ONLY subscriptions
    ADD CONSTRAINT fk_subscriptions_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY subscriptions
    ADD CONSTRAINT fk_subscriptions_pending_plan FOREIGN KEY (pending_plan_id) REFERENCES plans(id);

ALTER TABLE ONLY subscriptions
    ADD CONSTRAINT fk_subscriptions_plan FOREIGN KEY (plan_id) REFERENCES plans(id);

ALTER TABLE ONLY tokens
    ADD CONSTRAINT fk_tokens_credential FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE CASCADE;

ALTER TABLE ONLY tokens
    ADD CONSTRAINT fk_tokens_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY usage
    ADD CONSTRAINT fk_usage_credential FOREIGN KEY (credential_id) REFERENCES credentials(id);

ALTER TABLE ONLY usage
    ADD CONSTRAINT fk_usage_org FOREIGN KEY (org_id) REFERENCES orgs(id);


ALTER TABLE ONLY database_connections
    ADD CONSTRAINT fk_database_connections_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY database_connections
    ADD CONSTRAINT fk_database_connections_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY channels
    ADD CONSTRAINT fk_channels_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY channels
    ADD CONSTRAINT fk_channels_default_agent FOREIGN KEY (default_agent_id) REFERENCES agents(id) ON DELETE RESTRICT;
ALTER TABLE ONLY channels
    ADD CONSTRAINT fk_channels_external_connection FOREIGN KEY (external_connection_id) REFERENCES connections(id) ON DELETE SET NULL;
ALTER TABLE ONLY channels
    ADD CONSTRAINT fk_channels_creator FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE ONLY channel_members
    ADD CONSTRAINT fk_channel_members_channel FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY channel_members
    ADD CONSTRAINT fk_channel_members_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY sessions
    ADD CONSTRAINT fk_sessions_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY sessions
    ADD CONSTRAINT fk_sessions_channel FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY sessions
    ADD CONSTRAINT fk_sessions_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE RESTRICT;
ALTER TABLE ONLY sessions
    ADD CONSTRAINT fk_sessions_sandbox FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE SET NULL;
ALTER TABLE ONLY sessions
    ADD CONSTRAINT fk_sessions_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE ONLY session_participants
    ADD CONSTRAINT fk_session_participants_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;
ALTER TABLE ONLY session_participants
    ADD CONSTRAINT fk_session_participants_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY session_participants
    ADD CONSTRAINT fk_session_participants_inviter FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE ONLY session_events
    ADD CONSTRAINT fk_session_events_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY session_events
    ADD CONSTRAINT fk_session_events_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;
ALTER TABLE ONLY session_events
    ADD CONSTRAINT fk_session_events_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE RESTRICT;
ALTER TABLE ONLY session_events
    ADD CONSTRAINT fk_session_events_sandbox FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE SET NULL;
ALTER TABLE ONLY session_events
    ADD CONSTRAINT fk_session_events_actor_user FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE ONLY session_message_queue
    ADD CONSTRAINT fk_session_message_queue_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY session_message_queue
    ADD CONSTRAINT fk_session_message_queue_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;
ALTER TABLE ONLY session_message_queue
    ADD CONSTRAINT fk_session_message_queue_event FOREIGN KEY (session_event_id) REFERENCES session_events(id) ON DELETE CASCADE;

ALTER TABLE ONLY artifacts
    ADD CONSTRAINT fk_artifacts_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY artifacts
    ADD CONSTRAINT fk_artifacts_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL;
ALTER TABLE ONLY artifacts
    ADD CONSTRAINT fk_artifacts_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;
ALTER TABLE ONLY artifacts
    ADD CONSTRAINT fk_artifacts_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
