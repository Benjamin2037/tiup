# 高风险参数管理功能实现总结

## 实现内容

### 1. TiUP 侧实现

#### 新增文件
- `components/cluster/command/high_risk_params.go`: 高风险参数管理命令组实现

#### 修改文件
- `components/cluster/command/root.go`: 注册新的 `high-risk-params` 命令组

#### 功能特性

**命令结构：**
```bash
tiup cluster high-risk-params <subcommand> [flags]
```

**子命令：**

1. **`add`** - 添加高风险参数
   - 支持交互式模式（默认，当没有提供参数时）
   - 支持命令行模式（通过 flags 提供所有参数）
   - 参数：component, type, name, severity, description, check-modified, from-version, to-version, allowed-values

2. **`list`** - 列出所有高风险参数
   - 支持按组件过滤（--component）
   - 支持表格和 JSON 格式输出（--format）
   - 默认表格格式，可读性强

3. **`view`** - 查看单个参数详情
   - 显示参数的完整信息
   - 必需参数：component, type, name

4. **`remove`** - 删除高风险参数
   - 支持交互式模式（选择要删除的参数）
   - 支持命令行模式（直接指定参数）
   - 交互式模式下会列出所有可用参数供选择

5. **`edit`** - 编辑高风险参数
   - 支持交互式模式（逐步修改每个字段）
   - 支持命令行模式（只修改指定的字段）
   - 显示当前配置，允许保留或修改

#### 配置文件位置

优先级顺序：
1. 环境变量 `TIDB_UPGRADE_PRECHECK_HIGH_RISK_PARAMS_CONFIG`
2. `~/.tiup/storage/cluster/upgrade-precheck/high_risk_params.json`（TiUP 专用）
3. `~/.tiup/high_risk_params.json`（兼容旧位置）
4. `~/.tidb-upgrade-precheck/high_risk_params.json`（兼容独立工具）

### 2. tidb-upgrade-precheck 侧增强

#### 修改文件
- `cmd/precheck/high_risk_params_view.go`: 增强 view 命令，支持查看单个参数

#### 增强内容

**原功能：**
- `view` 命令只显示整个配置文件的 JSON

**新功能：**
- 支持查看单个参数（通过 --component, --type, --name 参数）
- 以格式化的方式显示参数详情
- 向后兼容：不提供参数时仍显示整个配置

**使用示例：**
```bash
# 查看整个配置
tidb-upgrade-precheck high-risk-params view --config <path>

# 查看单个参数
tidb-upgrade-precheck high-risk-params view \
  --config <path> \
  --component tidb \
  --type config \
  --name max-connections
```

## 使用示例

### 交互式添加参数

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
...
```

### 命令行添加参数

```bash
tiup cluster high-risk-params add \
  --component tidb \
  --type config \
  --name max-connections \
  --severity warning \
  --description "High connection count may cause resource exhaustion" \
  --check-modified \
  --from-version v7.0.0 \
  --to-version v7.5.0
```

### 列出参数

```bash
# 列出所有参数
tiup cluster high-risk-params list

# 按组件过滤
tiup cluster high-risk-params list --component tidb

# JSON 格式
tiup cluster high-risk-params list --format json
```

### 查看单个参数

```bash
tiup cluster high-risk-params view \
  --component tidb \
  --type config \
  --name max-connections
```

### 删除参数

```bash
# 交互式删除
tiup cluster high-risk-params remove

# 命令行删除
tiup cluster high-risk-params remove \
  --component tidb \
  --type config \
  --name max-connections
```

### 编辑参数

```bash
# 交互式编辑
tiup cluster high-risk-params edit \
  --component tidb \
  --type config \
  --name max-connections

# 命令行编辑
tiup cluster high-risk-params edit \
  --component tidb \
  --type config \
  --name max-connections \
  --severity error \
  --description "Updated description"
```

## 设计特点

1. **解耦设计**：TiUP 通过命令行调用 `tidb-upgrade-precheck`，保持工具独立性
2. **用户友好**：提供交互式和命令行两种模式，满足不同使用场景
3. **功能完整**：支持增删改查所有操作
4. **配置统一**：使用统一的配置文件位置，自动在 upgrade 命令中使用
5. **向后兼容**：保留现有的 `--high-risk-items` 标志

## 与现有功能集成

高风险参数配置会自动在 `tiup cluster upgrade` 命令中使用：

```bash
# 运行 precheck 时会自动加载高风险参数配置
tiup cluster upgrade my-cluster v8.0.0
```

配置文件位置优先级：
1. `--precheck-high-risk-params-config` 指定的路径
2. `~/.tiup/high_risk_params.json`
3. `~/.tiup/storage/cluster/upgrade-precheck/high_risk_params.json`

## 测试建议

1. **单元测试**：测试配置文件的读写、参数查找等功能
2. **集成测试**：测试完整的命令流程
3. **交互式测试**：验证交互式模式的用户体验
4. **兼容性测试**：验证与现有 upgrade 命令的集成

## 后续优化建议

1. **批量导入/导出**：支持从 JSON/YAML 文件批量导入
2. **参数模板**：提供常用参数的预设模板
3. **参数验证**：验证参数名和值的有效性
4. **版本管理**：支持配置文件的版本控制和回滚

