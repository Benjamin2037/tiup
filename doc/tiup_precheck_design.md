# TiUP Cluster Upgrade Precheck 设计实现说明

## 1. 概述

TiUP 的 `cluster upgrade` 命令集成了 `tidb-upgrade-precheck` 工具，用于在升级 TiDB 集群之前进行兼容性检查。该实现采用**命令行调用**的方式，实现了 TiUP 与 `tidb-upgrade-precheck` 的完全解耦。

## 2. 设计原则

### 2.1 解耦设计
- TiUP 不直接依赖 `tidb-upgrade-precheck` 的内部实现
- 通过命令行接口调用，保持工具间的独立性
- 其他工具（如 TiDB Operator）也可以采用相同方式集成

### 2.2 简单易用
- 默认行为：自动运行 precheck → 询问用户确认 → 执行升级
- 提供灵活的选项控制 precheck 行为
- 报告文件自动生成，支持多种格式

## 3. 命令接口

### 3.1 基本命令格式

```bash
tiup cluster upgrade <cluster-name> <version> [flags]
```

### 3.2 Precheck 相关标志

| 标志 | 说明 | 默认值 |
|------|------|--------|
| `--precheck` | 只运行 precheck，不执行升级 | false |
| `--without-precheck` | 跳过 precheck，直接升级 | false |
| `--precheck-output` | 报告格式 (text, markdown, html, json) | text |
| `--precheck-output-file` | 报告输出文件路径 | "" (输出到 stdout) |
| `--precheck-tidb-user` | TiDB SQL 用户名 | root |
| `--precheck-tidb-password` | TiDB SQL 密码 | "" |
| `--precheck-tidb-password-file` | TiDB SQL 密码文件路径 | "" |
| `--precheck-tidb-password-prompt` | 交互式输入 TiDB SQL 密码 | false |

### 3.3 使用场景

#### 场景 1: 默认行为（推荐）
```bash
tiup cluster upgrade <cluster-name> <version>
```
例如：
```bash
tiup cluster upgrade my-cluster v8.0.0
```
- 自动运行 precheck
- 显示报告
- 询问用户确认
- 执行升级

#### 场景 2: 只运行 precheck
```bash
tiup cluster upgrade <cluster-name> <version> --precheck
```
例如：
```bash
tiup cluster upgrade my-cluster v8.0.0 --precheck
```
- 运行 precheck
- 显示报告
- 退出，不执行升级

#### 场景 3: 跳过 precheck（不推荐）
```bash
tiup cluster upgrade <cluster-name> <version> --without-precheck
```
例如：
```bash
tiup cluster upgrade my-cluster v8.0.0 --without-precheck
```
- 跳过 precheck
- 直接执行升级

#### 场景 4: 指定报告格式和输出文件
```bash
tiup cluster upgrade <cluster-name> <version> \
  --precheck-output <format> \
  --precheck-output-file <file-path>
```
例如：
```bash
tiup cluster upgrade my-cluster v8.0.0 \
  --precheck-output markdown \
  --precheck-output-file ./precheck-report.md
```

## 4. 实现架构

### 4.1 拓扑信息获取

TiUP 通过 `cluster-name` 从本地元数据文件中获取集群拓扑信息：

1. **元数据文件位置**：
   ```
   ~/.tiup/storage/cluster/<cluster-name>/meta.yaml
   ```

2. **元数据文件结构**：
   ```yaml
   user: tidb
   tidb_version: v7.5.1
   last_ops_ver: v1.x.x
   topology:
     global:
       user: tidb
       deploy_dir: /tidb-deploy
       data_dir: /tidb-data
     tidb_servers:
       - host: 10.0.1.1
         port: 4000
     tikv_servers:
       - host: 10.0.1.2
         port: 20160
     pd_servers:
       - host: 10.0.1.3
         client_port: 2379
     # ... 其他组件配置
   ```

3. **获取流程**：
   ```go
   // 1. 通过 cluster-name 读取 meta.yaml
   metadata, err := spec.ClusterMetadata(clusterName)
   
   // 2. 从 metadata 中提取拓扑信息
   topo := metadata.GetTopology()
   
   // 3. 将拓扑信息序列化为 YAML
   topologyData, err := yaml.Marshal(topo)
   
   // 4. 写入临时文件供 tidb-upgrade-precheck 使用
   os.WriteFile(tmpTopologyFile, topologyData, 0644)
   ```

### 4.2 组件关系

```
┌─────────────────┐
│  TiUP Command   │
│  (upgrade.go)   │
└────────┬────────┘
         │
         │ 1. 通过 cluster-name 读取 meta.yaml
         │ 2. 提取拓扑信息
         │ 3. 创建临时 topology 文件
         │ 4. 构建命令参数
         │ 5. 调用 tidb-upgrade-precheck
         │
         ▼
┌─────────────────────────────┐
│  tidb-upgrade-precheck      │
│  (命令行工具)                │
│  - 读取 topology 文件        │
│  - 收集集群配置               │
│  - 执行分析                   │
│  - 生成报告文件               │
└────────┬────────────────────┘
         │
         │ 返回报告文件路径
         │
         ▼
┌─────────────────┐
│  报告文件       │
│  (text/md/html) │
└─────────────────┘
```

### 4.2 核心函数

#### `runParameterPrecheck`
主要实现函数，负责：
1. 获取集群元数据和拓扑
2. 创建临时 topology YAML 文件
3. 调用 `tidb-upgrade-precheck precheck` 命令
4. 从命令输出提取报告文件路径
5. 读取并显示报告内容（如需要）
6. 返回 `RiskReport` 结构

#### `buildPrecheckTiDBCredentials`
处理 TiDB 认证信息：
- 支持密码直接提供
- 支持从文件读取密码
- 支持交互式输入密码

## 5. 工作流程

### 5.1 详细流程

```
1. 用户执行命令
   └─> tiup cluster upgrade <cluster> <version> [flags]

2. 解析参数
   ├─> 检查 --precheck 标志
   ├─> 检查 --without-precheck 标志
   └─> 解析输出格式和文件路径

3. 获取集群信息
   ├─> 读取集群元数据 (spec.ClusterMetadata)
   ├─> 提取源版本信息
   └─> 获取拓扑结构 (spec.Specification)

4. 创建临时 topology 文件
   ├─> 将拓扑转换为 YAML 格式
   ├─> 添加源版本信息（如可用）
   └─> 保存到临时目录

5. 构建 tidb-upgrade-precheck 命令
   ├─> --topology-file: 临时 topology 文件路径
   ├─> --target-version: 目标版本
   ├─> --source-version: 源版本（如可用）
   ├─> --format: 输出格式
   ├─> --output-dir: 输出目录
   └─> --tidb-user/--tidb-password: 认证信息

6. 执行命令
   ├─> 调用 exec.CommandContext
   ├─> 捕获命令输出
   └─> 检查执行结果

7. 提取报告路径
   ├─> 从输出中提取 "Report generated successfully: {path}"
   └─> 如果提取失败，通过文件模式查找最新报告

8. 处理报告
   ├─> 如果未指定输出文件：读取并显示到 stdout
   └─> 如果指定了输出文件：报告已保存到指定位置

9. 返回结果
   └─> 返回 RiskReport（包含报告路径）

10. 用户确认（如果不是 --precheck 模式）
    └─> 询问是否继续升级

11. 执行升级（如果确认）
    └─> 调用 clusterUpgradeFunc
```

### 5.2 错误处理

- **命令执行失败**：返回错误，包含命令输出
- **报告文件未找到**：记录警告，显示命令输出
- **拓扑文件创建失败**：返回错误
- **版本信息缺失**：尝试从集群检测，失败则返回错误

## 6. 数据流

### 6.1 输入数据

- **集群名称**：用于获取集群元数据和拓扑
- **目标版本**：升级目标版本
- **拓扑信息**：从 TiUP 的集群元数据中获取
- **认证信息**：TiDB SQL 用户名和密码（可选）

### 6.2 输出数据

- **报告文件**：由 `tidb-upgrade-precheck` 生成
  - 格式：text, markdown, html, json
  - 位置：由 `--output-dir` 指定，默认当前目录
  - 文件名：`upgrade_precheck_report_{timestamp}.{ext}`
- **RiskReport 结构**：包含版本信息和报告路径

## 7. 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `TIDB_UPGRADE_PRECHECK_BIN` | tidb-upgrade-precheck 命令路径 | `tidb-upgrade-precheck` (从 PATH 查找) |
| `TIDB_UPGRADE_PRECHECK_KB_PATH` | 知识库路径 | `knowledge` (相对路径) |

## 8. 文件结构

### 8.1 临时文件

- **拓扑文件**：`/tmp/tiup-topology-{cluster-name}-{timestamp}.yaml`
  - 自动创建
  - 命令执行后自动清理

### 8.2 报告文件

- **默认位置**：当前工作目录
- **文件名格式**：`upgrade_precheck_report_{timestamp}.{ext}`
- **扩展名**：
  - text → `.txt`
  - markdown → `.md`
  - html → `.html`
  - json → `.json`

## 9. 集成优势

### 9.1 解耦性
- TiUP 不依赖 `tidb-upgrade-precheck` 的内部实现
- 只需确保命令在 PATH 中或通过环境变量指定
- `tidb-upgrade-precheck` 的更新不影响 TiUP

### 9.2 通用性
- 其他工具可以采用相同方式集成
- 只需调用命令，读取报告文件即可
- 支持所有 `tidb-upgrade-precheck` 支持的功能

### 9.3 可维护性
- 代码简洁，逻辑清晰
- 易于调试和测试
- 错误处理完善

## 10. 未来扩展

### 10.1 可能的改进
- 支持从报告文件中解析风险项数量（用于自动决策）
- 支持报告文件的自动清理策略
- 支持报告文件的归档和版本管理

### 10.2 与其他工具的集成
- TiDB Operator 可以采用相同方式调用
- 可以集成到 CI/CD 流程中
- 可以集成到监控和告警系统中

## 11. 代码示例

### 11.1 核心调用代码

```go
func runParameterPrecheck(ctx context.Context, clusterName, targetVersion string, 
    logger *logprinter.Logger, format string, outputPath string, 
    creds tidbSQLCredentials, cm *manager.Manager) (*RiskReport, error) {
    
    // 1. 获取集群拓扑
    metadata, _ := spec.ClusterMetadata(clusterName)
    topo := metadata.GetTopology()
    
    // 2. 创建临时 topology 文件
    tmpTopologyFile := createTempTopologyFile(topo, sourceVersion)
    defer os.Remove(tmpTopologyFile)
    
    // 3. 构建命令
    cmd := exec.CommandContext(ctx, "tidb-upgrade-precheck", "precheck",
        "--topology-file", tmpTopologyFile,
        "--target-version", targetVersion,
        "--format", format,
        "--output-dir", outputDir,
    )
    
    // 4. 执行命令
    cmdOutput, err := cmd.CombinedOutput()
    
    // 5. 提取报告路径
    reportPath := extractReportPath(string(cmdOutput), outputDir, format)
    
    // 6. 读取并显示报告（如需要）
    if outputPath == "" && reportPath != "" {
        content, _ := os.ReadFile(reportPath)
        fmt.Println(string(content))
    }
    
    // 7. 返回结果
    return &RiskReport{
        SourceVersion: sourceVersion,
        TargetVersion: targetVersion,
        ReportPath: reportPath,
    }, nil
}
```

## 12. 测试建议

### 12.1 单元测试
- 测试 topology 文件创建
- 测试命令参数构建
- 测试报告路径提取
- 测试错误处理

### 12.2 集成测试
- 测试完整的 precheck 流程
- 测试不同输出格式
- 测试错误场景

## 13. 注意事项

1. **命令可用性**：确保 `tidb-upgrade-precheck` 命令在 PATH 中或通过环境变量指定
2. **知识库路径**：确保知识库文件已生成并位于正确位置
3. **临时文件清理**：临时 topology 文件会在函数返回后自动清理
4. **报告文件管理**：报告文件不会自动清理，需要用户手动管理
5. **认证安全**：密码通过命令行参数传递，注意日志记录时的安全性

