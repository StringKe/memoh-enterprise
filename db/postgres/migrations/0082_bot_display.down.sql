-- 0082_bot_display

ALTER TABLE bots
  DROP COLUMN IF EXISTS display_enabled;
