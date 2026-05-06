-- name: GetSettingsByBotID :one
SELECT
  bots.id AS bot_id,
  bots.language,
  bots.reasoning_enabled,
  bots.reasoning_effort,
  bots.heartbeat_enabled,
  bots.heartbeat_interval,
  bots.heartbeat_prompt,
  bots.compaction_enabled,
  bots.compaction_threshold,
  bots.compaction_ratio,
  bots.timezone,
  bots.chat_model_id,
  bots.heartbeat_model_id,
  bots.compaction_model_id,
  bots.title_model_id,
  bots.search_provider_id,
  bots.memory_provider_id,
  bots.image_model_id,
  bots.tts_model_id,
  bots.transcription_model_id,
  bots.browser_context_id,
  bots.persist_full_tool_results,
  bots.show_tool_calls_in_im,
  bots.tool_approval_config,
  bots.overlay_provider,
  bots.overlay_enabled,
  bots.overlay_config,
  bots.group_id,
  bots.settings_override_mask,
  bot_group_settings.timezone AS group_timezone,
  bot_group_settings.language AS group_language,
  bot_group_settings.reasoning_enabled AS group_reasoning_enabled,
  bot_group_settings.reasoning_effort AS group_reasoning_effort,
  bot_group_settings.chat_model_id AS group_chat_model_id,
  bot_group_settings.search_provider_id AS group_search_provider_id,
  bot_group_settings.memory_provider_id AS group_memory_provider_id,
  bot_group_settings.heartbeat_enabled AS group_heartbeat_enabled,
  bot_group_settings.heartbeat_interval AS group_heartbeat_interval,
  bot_group_settings.heartbeat_prompt AS group_heartbeat_prompt,
  bot_group_settings.heartbeat_model_id AS group_heartbeat_model_id,
  bot_group_settings.compaction_enabled AS group_compaction_enabled,
  bot_group_settings.compaction_threshold AS group_compaction_threshold,
  bot_group_settings.compaction_ratio AS group_compaction_ratio,
  bot_group_settings.compaction_model_id AS group_compaction_model_id,
  bot_group_settings.title_model_id AS group_title_model_id,
  bot_group_settings.image_model_id AS group_image_model_id,
  bot_group_settings.discuss_probe_model_id AS group_discuss_probe_model_id,
  bot_group_settings.tts_model_id AS group_tts_model_id,
  bot_group_settings.transcription_model_id AS group_transcription_model_id,
  bot_group_settings.browser_context_id AS group_browser_context_id,
  bot_group_settings.persist_full_tool_results AS group_persist_full_tool_results,
  bot_group_settings.show_tool_calls_in_im AS group_show_tool_calls_in_im,
  bot_group_settings.tool_approval_config AS group_tool_approval_config,
  bot_group_settings.overlay_provider AS group_overlay_provider,
  bot_group_settings.overlay_enabled AS group_overlay_enabled,
  bot_group_settings.overlay_config AS group_overlay_config
FROM bots
LEFT JOIN bot_group_settings ON bot_group_settings.group_id = bots.group_id
LEFT JOIN models AS chat_models ON chat_models.id = bots.chat_model_id
LEFT JOIN models AS heartbeat_models ON heartbeat_models.id = bots.heartbeat_model_id
LEFT JOIN models AS compaction_models ON compaction_models.id = bots.compaction_model_id
LEFT JOIN models AS title_models ON title_models.id = bots.title_model_id
LEFT JOIN models AS image_models ON image_models.id = bots.image_model_id
LEFT JOIN search_providers ON search_providers.id = bots.search_provider_id
LEFT JOIN memory_providers ON memory_providers.id = bots.memory_provider_id
LEFT JOIN models AS tts_models ON tts_models.id = bots.tts_model_id
LEFT JOIN models AS transcription_models ON transcription_models.id = bots.transcription_model_id
LEFT JOIN browser_contexts ON browser_contexts.id = bots.browser_context_id
WHERE bots.id = $1;

-- name: UpsertBotSettings :one
WITH updated AS (
  UPDATE bots
  SET language = sqlc.arg(language),
      reasoning_enabled = sqlc.arg(reasoning_enabled),
      reasoning_effort = sqlc.arg(reasoning_effort),
      heartbeat_enabled = sqlc.arg(heartbeat_enabled),
      heartbeat_interval = sqlc.arg(heartbeat_interval),
      heartbeat_prompt = sqlc.arg(heartbeat_prompt),
      compaction_enabled = sqlc.arg(compaction_enabled),
      compaction_threshold = sqlc.arg(compaction_threshold),
      compaction_ratio = sqlc.arg(compaction_ratio),
      timezone = CASE WHEN sqlc.arg(timezone_present)::boolean THEN sqlc.narg(timezone)::text ELSE bots.timezone END,
      chat_model_id = CASE WHEN sqlc.arg(chat_model_id_present)::boolean THEN sqlc.narg(chat_model_id)::uuid ELSE bots.chat_model_id END,
      heartbeat_model_id = CASE WHEN sqlc.arg(heartbeat_model_id_present)::boolean THEN sqlc.narg(heartbeat_model_id)::uuid ELSE bots.heartbeat_model_id END,
      compaction_model_id = CASE WHEN sqlc.arg(compaction_model_id_present)::boolean THEN sqlc.narg(compaction_model_id)::uuid ELSE bots.compaction_model_id END,
      title_model_id = CASE WHEN sqlc.arg(title_model_id_present)::boolean THEN sqlc.narg(title_model_id)::uuid ELSE bots.title_model_id END,
      search_provider_id = CASE WHEN sqlc.arg(search_provider_id_present)::boolean THEN sqlc.narg(search_provider_id)::uuid ELSE bots.search_provider_id END,
      memory_provider_id = CASE WHEN sqlc.arg(memory_provider_id_present)::boolean THEN sqlc.narg(memory_provider_id)::uuid ELSE bots.memory_provider_id END,
      image_model_id = CASE WHEN sqlc.arg(image_model_id_present)::boolean THEN sqlc.narg(image_model_id)::uuid ELSE bots.image_model_id END,
      tts_model_id = CASE WHEN sqlc.arg(tts_model_id_present)::boolean THEN sqlc.narg(tts_model_id)::uuid ELSE bots.tts_model_id END,
      transcription_model_id = CASE WHEN sqlc.arg(transcription_model_id_present)::boolean THEN sqlc.narg(transcription_model_id)::uuid ELSE bots.transcription_model_id END,
      browser_context_id = CASE WHEN sqlc.arg(browser_context_id_present)::boolean THEN sqlc.narg(browser_context_id)::uuid ELSE bots.browser_context_id END,
      persist_full_tool_results = sqlc.arg(persist_full_tool_results),
      show_tool_calls_in_im = sqlc.arg(show_tool_calls_in_im),
      tool_approval_config = sqlc.arg(tool_approval_config),
      overlay_provider = sqlc.arg(overlay_provider),
      overlay_enabled = sqlc.arg(overlay_enabled),
      overlay_config = sqlc.arg(overlay_config),
      updated_at = now()
  WHERE bots.id = sqlc.arg(id)
  RETURNING bots.id, bots.language, bots.reasoning_enabled, bots.reasoning_effort, bots.heartbeat_enabled, bots.heartbeat_interval, bots.heartbeat_prompt, bots.compaction_enabled, bots.compaction_threshold, bots.compaction_ratio, bots.timezone, bots.chat_model_id, bots.heartbeat_model_id, bots.compaction_model_id, bots.title_model_id, bots.image_model_id, bots.search_provider_id, bots.memory_provider_id, bots.tts_model_id, bots.transcription_model_id, bots.browser_context_id, bots.persist_full_tool_results, bots.show_tool_calls_in_im, bots.tool_approval_config, bots.overlay_provider, bots.overlay_enabled, bots.overlay_config
)
SELECT
  updated.id AS bot_id,
  updated.language,
  updated.reasoning_enabled,
  updated.reasoning_effort,
  updated.heartbeat_enabled,
  updated.heartbeat_interval,
  updated.heartbeat_prompt,
  updated.compaction_enabled,
  updated.compaction_threshold,
  updated.compaction_ratio,
  updated.timezone,
  chat_models.id AS chat_model_id,
  heartbeat_models.id AS heartbeat_model_id,
  compaction_models.id AS compaction_model_id,
  title_models.id AS title_model_id,
  search_providers.id AS search_provider_id,
  memory_providers.id AS memory_provider_id,
  image_models.id AS image_model_id,
  tts_models.id AS tts_model_id,
  transcription_models.id AS transcription_model_id,
  browser_contexts.id AS browser_context_id,
  updated.persist_full_tool_results,
  updated.show_tool_calls_in_im,
  updated.tool_approval_config,
  updated.overlay_provider,
  updated.overlay_enabled,
  updated.overlay_config
FROM updated
LEFT JOIN models AS chat_models ON chat_models.id = updated.chat_model_id
LEFT JOIN models AS heartbeat_models ON heartbeat_models.id = updated.heartbeat_model_id
LEFT JOIN models AS compaction_models ON compaction_models.id = updated.compaction_model_id
LEFT JOIN models AS title_models ON title_models.id = updated.title_model_id
LEFT JOIN models AS image_models ON image_models.id = updated.image_model_id
LEFT JOIN search_providers ON search_providers.id = updated.search_provider_id
LEFT JOIN memory_providers ON memory_providers.id = updated.memory_provider_id
LEFT JOIN models AS tts_models ON tts_models.id = updated.tts_model_id
LEFT JOIN models AS transcription_models ON transcription_models.id = updated.transcription_model_id
LEFT JOIN browser_contexts ON browser_contexts.id = updated.browser_context_id;

-- name: DeleteSettingsByBotID :exec
UPDATE bots
SET language = 'auto',
    reasoning_enabled = false,
    reasoning_effort = 'medium',
    heartbeat_enabled = false,
    heartbeat_interval = 30,
    heartbeat_prompt = '',
    compaction_enabled = false,
    compaction_threshold = 100000,
    compaction_ratio = 80,
    chat_model_id = NULL,
    heartbeat_model_id = NULL,
    compaction_model_id = NULL,
    title_model_id = NULL,
    image_model_id = NULL,
    search_provider_id = NULL,
    memory_provider_id = NULL,
    tts_model_id = NULL,
    transcription_model_id = NULL,
    browser_context_id = NULL,
    persist_full_tool_results = false,
    show_tool_calls_in_im = false,
    tool_approval_config = '{"enabled":false,"write":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"edit":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"exec":{"require_approval":false,"bypass_commands":[],"force_review_commands":[]}}'::jsonb,
    overlay_provider = '',
    overlay_enabled = false,
    overlay_config = '{}'::jsonb,
    settings_override_mask = CASE
      WHEN group_id IS NULL THEN '{}'::jsonb
      ELSE jsonb_build_object(
        'timezone', false,
        'language', false,
        'reasoning_enabled', false,
        'reasoning_effort', false,
        'chat_model_id', false,
        'search_provider_id', false,
        'memory_provider_id', false,
        'heartbeat_enabled', false,
        'heartbeat_interval', false,
        'heartbeat_prompt', false,
        'heartbeat_model_id', false,
        'compaction_enabled', false,
        'compaction_threshold', false,
        'compaction_ratio', false,
        'compaction_model_id', false,
        'title_model_id', false,
        'image_model_id', false,
        'discuss_probe_model_id', false,
        'tts_model_id', false,
        'transcription_model_id', false,
        'browser_context_id', false,
        'persist_full_tool_results', false,
        'show_tool_calls_in_im', false,
        'tool_approval_config', false,
        'overlay_provider', false,
        'overlay_enabled', false,
        'overlay_config', false
      )
    END,
    updated_at = now()
WHERE id = $1;

-- name: UpdateBotSettingsOverrideMask :exec
UPDATE bots
SET settings_override_mask = sqlc.arg(settings_override_mask)::jsonb,
    updated_at = now()
WHERE id = sqlc.arg(id);
