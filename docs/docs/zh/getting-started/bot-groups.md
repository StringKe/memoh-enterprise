# Bot Group

Bot Group 为多个 bot 定义共享默认设置。一个 bot 可以归属到一个 group，并在字段未被本地覆盖时继承 group 设置。

## 继承模型

- group settings 是默认值，不是创建 bot 时复制出来的快照。
- bot settings 按字段覆盖 group 默认值。
- 当字段在 override mask 中标记时，本地空值也算明确的 bot 覆盖。
- 恢复继承会移除所选字段的 bot 覆盖，并重新从 group 解析有效值。
- 如果 bot 没有 group 默认值，server 回退到系统默认值。

## 在 Group 中创建 Bot

创建 bot 时先选择 Bot Group。只有用户明确选择了本地 model 或 memory provider 时，创建后才写入对应字段的 override mask。未选择本地值时，bot 继承 group 默认值。

## 管理 API

Web UI 使用 ConnectRPC 管理 API 处理 Bot Group CRUD、group settings、bot assignment、effective settings 和 restore inheritance。OpenAPI 管理端点不是 enterprise 目标范围。
