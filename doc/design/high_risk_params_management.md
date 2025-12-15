# TiUP 高风险参数管理设计

## 1. 概述

在 TiUP 侧提供友好的命令行接口来管理高风险参数配置，支持用户通过交互式或命令行方式添加、查看、编辑和删除高风险参数。

## 2. 设计目标

1. **用户友好**：提供交互式和命令行两种模式
2. **功能完整**：支持增删改查所有操作
3. **解耦设计**：通过调用 `tidb-upgrade-precheck` 命令实现，保持工具间独立性
4. **配置统一**：使用统一的配置文件位置 `~/.tiup/storage/cluster/upgrade-precheck/high_risk_params.json`

## 3. 命令结构

### 3.1 主命令

在 `tiup cluster` 下添加新的子命令组：

```bash
tiup cluster high-risk-params <subcommand> [flags]
```

### 3.2 子命令

| 子命令 | 功能 | 交互式支持 |
|--------|------|-----------|
| `add` | 添加高风险参数 | ✅ |
| `list` | 列出所有高风险参数 | ❌ |
| `view` | 查看单个参数详情 | ❌ |
| `remove` | 删除高风险参数 | ✅ |
| `edit` | 编辑高风险参数 | ✅ |

## 4. 详细设计

### 4.1 添加命令 (`add`)

#### 交互式模式（默认）

```bash
tiup cluster high-risk-params add
# 或
tiup cluster high-risk-params add --interactive
```

交互流程：
1. 选择组件（tidb/pd/tikv/tiflash）
2. 选择参数类型（config/system_variable，仅 TiDB 支持 system_variable）
3. 输入参数名
4. 选择严重程度（error/warning/info）
5. 输入描述（可选）
6. 是否只检查修改过的参数（y/n）
7. 输入起始版本（可选）
8. 输入结束版本（可选）
9. 是否指定允许值（y/n），如果是则逐个输入

#### 命令行模式

```bash
tiup cluster high-risk-params add \
  --component tidb \
  --type config \
  --name max-connections \
  --severity warning \
  --description "High connection count may cause resource exhaustion" \
  --check-modified \
  --from-version v7.0.0 \
  --to-version v7.5.0 \
  --allowed-values 1000,2000,3000
```

**参数说明：**
- `--component` (必需): 组件名称 (tidb, pd, tikv, tiflash)
- `--type` (必需): 参数类型 (config, system_variable)
- `--name` (必需): 参数名称
- `--severity` (必需): 严重程度 (error, warning, info)
- `--description` (可选): 参数描述
- `--check-modified` (可选): 仅检查修改过的参数
- `--from-version` (可选): 起始版本
- `--to-version` (可选): 结束版本
- `--allowed-values` (可选): 允许的值（逗号分隔）
- `--interactive, -i` (可选): 使用交互式模式

### 4.2 列出命令 (`list`)

```bash
# 列出所有参数
tiup cluster high-risk-params list

# 按组件过滤
tiup cluster high-risk-params list --component tidb

# JSON 格式输出
tiup cluster high-risk-params list --format json

# 按组件过滤并 JSON 输出
tiup cluster high-risk-params list --component pd --format json
```

**参数说明：**
- `--component` (可选): 过滤组件 (tidb, pd, tikv, tiflash)
- `--format` (可选): 输出格式 (table, json)，默认 table

**输出示例（表格格式）：**

```
=== TiDB ===

Config Parameters:
  max-connections (config)
    Severity: warning
    Description: High connection count may cause resource exhaustion
    From Version: v7.0.0
    To Version: v7.5.0
    Check Modified: true

System Variables:
  tidb_mem_quota_query (system_variable)
    Severity: error
    Description: Query memory quota limit

=== PD ===

  schedule.max-merge-region-size (config)
    Severity: warning
    Description: Large merge region size may affect performance
```

### 4.3 查看命令 (`view`)

```bash
tiup cluster high-risk-params view \
  --component tidb \
  --type config \
  --name max-connections
```

**参数说明：**
- `--component` (必需): 组件名称
- `--type` (必需): 参数类型
- `--name` (必需): 参数名称

**输出示例：**

```
Parameter: max-connections
Component: tidb
Type: config
Severity: warning
Description: High connection count may cause resource exhaustion
From Version: v7.0.0
To Version: v7.5.0
Check Modified: true
Allowed Values: [1000, 2000, 3000]
```

### 4.4 删除命令 (`remove`)

#### 交互式模式

```bash
tiup cluster high-risk-params remove
# 或
tiup cluster high-risk-params remove --interactive
```

交互流程：
1. 选择组件
2. 选择参数类型
3. 输入参数名（支持自动补全）
4. 确认删除

#### 命令行模式

```bash
tiup cluster high-risk-params remove \
  --component tidb \
  --type config \
  --name max-connections
```

**参数说明：**
- `--component` (必需): 组件名称
- `--type` (必需): 参数类型
- `--name` (必需): 参数名称
- `--interactive, -i` (可选): 使用交互式模式

### 4.5 编辑命令 (`edit`)

#### 交互式模式

```bash
tiup cluster high-risk-params edit \
  --component tidb \
  --type config \
  --name max-connections
```

交互流程：
1. 显示当前配置
2. 逐个字段询问是否修改（回车保持原值）
3. 确认修改

#### 命令行模式

```bash
tiup cluster high-risk-params edit \
  --component tidb \
  --type config \
  --name max-connections \
  --severity error \
  --description "Updated description"
```

**参数说明：**
- `--component` (必需): 组件名称
- `--type` (必需): 参数类型
- `--name` (必需): 参数名称
- 其他参数与 `add` 命令相同，用于更新对应字段

## 5. 实现架构

### 5.1 命令调用流程

```
┌─────────────────────┐
│  TiUP Command       │
│  high-risk-params   │
└──────────┬──────────┘
           │
           │ 1. 解析参数
           │ 2. 验证输入
           │ 3. 定位配置文件
           │ 4. 调用 tidb-upgrade-precheck
           │
           ▼
┌─────────────────────────────┐
│  tidb-upgrade-precheck       │
│  high-risk-params <action>   │
│  - 读取/写入配置文件         │
│  - 执行相应操作              │
└──────────┬──────────────────┘
           │
           │ 返回结果
           │
           ▼
┌─────────────────────┐
│  配置文件           │
│  ~/.tiup/storage/   │
│  cluster/upgrade-   │
│  precheck/          │
│  high_risk_params.  │
│  json               │
└─────────────────────┘
```

### 5.2 配置文件位置

优先级顺序：
1. 环境变量 `TIDB_UPGRADE_PRECHECK_HIGH_RISK_PARAMS_CONFIG`
2. `~/.tiup/storage/cluster/upgrade-precheck/high_risk_params.json`（TiUP 专用）
3. `~/.tiup/high_risk_params.json`（兼容旧位置）
4. `~/.tidb-upgrade-precheck/high_risk_params.json`（兼容独立工具）

### 5.3 核心函数设计

#### `newHighRiskParamsCmd()`
创建高风险参数管理命令组

#### `newHighRiskParamsAddCmd()`
- 支持交互式和命令行模式
- 调用 `tidb-upgrade-precheck high-risk-params add`

#### `newHighRiskParamsListCmd()`
- 支持表格和 JSON 格式
- 支持组件过滤
- 调用 `tidb-upgrade-precheck high-risk-params list`

#### `newHighRiskParamsViewCmd()`
- 显示单个参数详情
- 调用 `tidb-upgrade-precheck high-risk-params view`

#### `newHighRiskParamsRemoveCmd()`
- 支持交互式确认
- 调用 `tidb-upgrade-precheck high-risk-params remove`

#### `newHighRiskParamsEditCmd()`
- 支持交互式和命令行模式
- 调用 `tidb-upgrade-precheck high-risk-params edit`

### 5.4 交互式辅助函数

复用 TiUP 的 `tui` 包：
- `tui.PromptForConfirm()`: 确认提示
- `tui.PromptForInput()`: 输入提示
- `tui.PromptForSelect()`: 选择提示

## 6. 使用示例

### 示例 1: 交互式添加高风险参数

```bash
$ tiup cluster high-risk-params add

Select component:
  * [1] tidb
    [2] pd
    [3] tikv
    [4] tiflash
Select option [1]: 1

Select parameter type:
  * [1] config
    [2] system_variable
Select option [1]: 1

Enter parameter name: max-connections

Select severity:
  * [1] error
    [2] warning
    [3] info
Select option [2]: 2

Enter description (optional): High connection count may cause resource exhaustion

Only check if parameter is modified from default? (y/n) [n]: y

From version (e.g., v7.0.0, optional): v7.0.0

To version (e.g., v7.5.0, optional): v7.5.0

Specify allowed values? (y/n) [n]: n

Successfully added high-risk parameter: tidb/config/max-connections
Configuration saved to: ~/.tiup/storage/cluster/upgrade-precheck/high_risk_params.json
```

### 示例 2: 命令行批量添加

```bash
# 添加 TiDB 配置参数
tiup cluster high-risk-params add \
  --component tidb \
  --type config \
  --name max-connections \
  --severity warning \
  --description "High connection count" \
  --check-modified \
  --from-version v7.0.0 \
  --to-version v7.5.0

# 添加 TiDB 系统变量
tiup cluster high-risk-params add \
  --component tidb \
  --type system_variable \
  --name tidb_mem_quota_query \
  --severity error \
  --description "Query memory quota limit"

# 添加 PD 参数
tiup cluster high-risk-params add \
  --component pd \
  --type config \
  --name schedule.max-merge-region-size \
  --severity warning
```

### 示例 3: 查看和编辑

```bash
# 列出所有参数
tiup cluster high-risk-params list

# 查看 TiDB 参数
tiup cluster high-risk-params list --component tidb

# 查看单个参数详情
tiup cluster high-risk-params view \
  --component tidb \
  --type config \
  --name max-connections

# 编辑参数（交互式）
tiup cluster high-risk-params edit \
  --component tidb \
  --type config \
  --name max-connections

# 编辑参数（命令行）
tiup cluster high-risk-params edit \
  --component tidb \
  --type config \
  --name max-connections \
  --severity error \
  --description "Updated description"
```

### 示例 4: 删除参数

```bash
# 交互式删除
tiup cluster high-risk-params remove

# 命令行删除
tiup cluster high-risk-params remove \
  --component tidb \
  --type config \
  --name max-connections
```

## 7. 与现有功能的集成

### 7.1 与 upgrade 命令集成

高风险参数配置会自动在 `tiup cluster upgrade` 命令中使用：

```bash
# 运行 precheck 时会自动加载高风险参数配置
tiup cluster upgrade my-cluster v8.0.0

# 也可以显式指定配置文件
tiup cluster upgrade my-cluster v8.0.0 \
  --precheck-high-risk-params-config ~/.tiup/storage/cluster/upgrade-precheck/high_risk_params.json
```

### 7.2 向后兼容

保留现有的 `--high-risk-items` 标志（在 upgrade 命令中），但推荐使用新的子命令：

```bash
# 旧方式（仍然支持）
tiup cluster upgrade my-cluster v8.0.0 --high-risk-items --list
tiup cluster upgrade my-cluster v8.0.0 --high-risk-items --edit file.json

# 新方式（推荐）
tiup cluster high-risk-params list
tiup cluster high-risk-params add ...
```

## 8. 错误处理

1. **配置文件不存在**：自动创建空配置文件
2. **参数已存在**：`add` 命令提示是否覆盖或使用 `edit` 命令
3. **参数不存在**：`view`/`edit`/`remove` 命令提示参数不存在
4. **无效输入**：提供清晰的错误信息和修复建议
5. **二进制不可用**：提示安装或配置 `tidb-upgrade-precheck`

## 9. 优势

1. **用户友好**：交互式模式降低使用门槛
2. **功能完整**：支持所有 CRUD 操作
3. **解耦设计**：通过命令行调用，保持工具独立性
4. **配置统一**：使用统一的配置文件位置
5. **向后兼容**：保留现有接口，平滑迁移

## 10. 未来扩展

1. **批量导入/导出**：支持从 JSON/YAML 文件批量导入
2. **参数模板**：提供常用参数的预设模板
3. **参数验证**：验证参数名和值的有效性
4. **版本管理**：支持配置文件的版本控制和回滚
5. **参数建议**：基于历史升级经验推荐高风险参数

