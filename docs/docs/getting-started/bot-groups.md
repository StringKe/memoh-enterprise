# Bot Groups

Bot Groups define shared defaults for multiple bots. A bot can belong to one group and inherit group settings until a field is explicitly overridden on the bot.

## Inheritance Model

- Group settings are defaults, not copies.
- Bot settings override group defaults per field.
- Empty local values count as explicit bot overrides when the field is marked in the override mask.
- Restoring inheritance removes the bot override for selected fields and resolves the value from the group again.
- If a bot has no group default for a field, the server falls back to the system default.

## Creating Bots In A Group

When creating a bot, choose a Bot Group before saving. Initial model or memory settings are written with an override mask only when the user selected a local value. If no local value is selected, the bot inherits the group default.

## Management API

The Web UI uses ConnectRPC management APIs for Bot Group CRUD, group settings, bot assignment, effective settings, and restore-inheritance actions. OpenAPI management endpoints are not part of the enterprise target.
