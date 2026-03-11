# Drop 协议与 3x-ui 集成说明

## 一、3x-ui 与 Xray-core 的接口通信方式

### 1.1 现有接口架构

3x-ui 通过 **gRPC** 接口与 Xray-core 通信：

```
┌─────────────────┐         gRPC          ┌─────────────────┐
│     3x-ui       │ ────────────────────→ │   Xray-core     │
│   (Go Client)   │                        │  (gRPC Server)   │
└─────────────────┘                        └─────────────────┘
```

### 1.2 主要接口服务

3x-ui 使用以下 gRPC 服务：

1. **HandlerService** (`app/proxyman/command`)
   - `AddInbound()` - 动态添加入站配置
   - `RemoveInbound()` - 动态删除入站配置
   - `AlterInbound()` - 修改入站配置（添加/删除用户）

2. **StatsService** (`app/stats/command`)
   - `QueryStats()` - 查询流量统计信息

### 1.3 接口调用位置

在 3x-ui 代码中，接口调用主要在：

- **`xray/api.go`**：gRPC 客户端封装
  - `XrayAPI.Init()` - 初始化连接
  - `XrayAPI.AddInbound()` - 添加入站
  - `XrayAPI.DelInbound()` - 删除入站
  - `XrayAPI.AddUser()` - 添加用户
  - `XrayAPI.RemoveUser()` - 删除用户
  - `XrayAPI.GetTraffic()` - 获取流量统计

- **`web/service/xray.go`**：Xray 服务管理
  - `GetXrayConfig()` - 生成 Xray 配置
  - `RestartXray()` - 重启 Xray（应用配置）

- **`web/service/inbound.go`**：入站管理
  - 使用 `XrayAPI` 动态添加/删除入站和用户

## 二、Drop 协议的集成方式

### 2.1 不需要新增 API 接口

**重要**：drop 协议作为标准的 outbound 协议，**不需要**新增任何 API 接口。

原因：
1. drop 协议是 outbound 协议，不是 inbound 协议
2. outbound 配置通过配置文件管理，不需要动态 API
3. 路由规则通过配置文件管理，不需要动态 API
4. 3x-ui 通过生成配置文件并重启 Xray 来应用 outbound 配置

### 2.2 集成流程

```
用户操作（3x-ui前端）
    ↓
保存配置到数据库
    ↓
生成 Xray 配置文件（包含 drop outbound）
    ↓
重启 Xray（应用新配置）
    ↓
Xray 加载 drop 协议并生效
```

### 2.3 配置生成位置

drop 协议的配置在以下位置生成：

1. **`web/service/xray.go`** - `GetXrayConfig()`
   - 从 `xrayTemplateConfig` 读取模板配置
   - 模板配置中包含 outbound 配置（包括 drop 协议）

2. **`web/service/xray_setting.go`** - `SaveXraySetting()`
   - 保存用户编辑的 Xray 模板配置
   - 用户可以在模板中添加 drop 协议的 outbound

3. **`web/service/outbound.go`**（如果未来需要）
   - 可以添加专门的 outbound 管理服务
   - 但目前 outbound 通过模板配置管理

## 三、3x-ui 需要做的修改

### 3.1 前端修改

#### 3.1.1 添加 drop 协议选项

**文件**：`web/assets/js/model/outbound.js`

在 `Protocols` 枚举中添加：
```javascript
const Protocols = {
    Freedom: "freedom",
    Blackhole: "blackhole",
    Drop: "drop",  // 新增
    // ... 其他协议
};
```

#### 3.1.2 添加 drop 协议设置类

**文件**：`web/assets/js/model/outbound.js`

添加 `DropSettings` 类：
```javascript
Outbound.DropSettings = class extends CommonClass {
    constructor(lossPercent = 0, direction = "all") {
        super();
        this.lossPercent = lossPercent;  // 0-100
        this.direction = direction;      // "upload"/"download"/"all"
    }
    
    static fromJson(json = {}) {
        return new Outbound.DropSettings(
            json.lossPercent || 0,
            json.direction || "all"
        );
    }
    
    toJson() {
        return {
            lossPercent: this.lossPercent,
            direction: this.direction
        };
    }
};
```

#### 3.1.3 更新协议设置获取逻辑

在 `Outbound.getSettings()` 和 `Outbound.fromJson()` 中添加 drop 协议的处理。

#### 3.1.4 添加前端 UI

**文件**：`web/html/form/outbound.html`

添加 drop 协议的配置表单：
```html
<!-- drop settings -->
<template v-if="outbound.protocol === Protocols.Drop">
    <a-form-item label="丢包率 (%)">
        <a-input-number 
            v-model="outbound.settings.lossPercent" 
            :min="0" 
            :max="100" 
            :precision="1">
        </a-input-number>
    </a-form-item>
    
    <a-form-item label="丢包方向">
        <a-select v-model="outbound.settings.direction">
            <a-select-option value="upload">上行 (Upload)</a-select-option>
            <a-select-option value="download">下行 (Download)</a-select-option>
            <a-select-option value="all">全部 (All)</a-select-option>
        </a-select>
    </a-form-item>
</template>
```

### 3.2 后端修改

#### 3.2.1 配置验证（可选）

**文件**：`web/service/xray_setting.go`

在 `CheckXrayConfig()` 中可以添加 drop 协议的配置验证，但 Xray-core 本身会验证，所以这一步是可选的。

#### 3.2.2 无需修改 API 接口

**重要**：不需要修改任何 API 接口，因为：
- drop 协议配置通过 JSON 配置文件传递
- 3x-ui 通过生成配置文件并重启 Xray 来应用配置
- 不需要动态添加/删除 outbound 的 API

## 四、配置示例

### 4.1 在 3x-ui 中配置 drop 协议

用户在 3x-ui 的 Xray 设置页面添加 outbound：

```json
{
  "outbounds": [
    {
      "tag": "drop-uplink-30",
      "protocol": "drop",
      "settings": {
        "lossPercent": 30.0,
        "direction": "upload"
      }
    },
    {
      "tag": "drop-downlink-20",
      "protocol": "drop",
      "settings": {
        "lossPercent": 20.0,
        "direction": "download"
      }
    }
  ],
  "routing": {
    "rules": [
      {
        "type": "field",
        "ip": ["1.2.3.0/24"],
        "inboundTag": ["inbound1"],
        "user": ["user1@example.com"],
        "outboundTag": "drop-uplink-30"
      }
    ]
  }
}
```

### 4.2 配置应用流程

1. **用户编辑配置**：在 3x-ui 前端编辑 Xray 模板配置
2. **保存配置**：调用 `XraySettingService.SaveXraySetting()`
3. **生成最终配置**：`XrayService.GetXrayConfig()` 读取模板并添加 inbounds
4. **重启 Xray**：`XrayService.RestartXray()` 应用新配置
5. **Xray 加载**：Xray-core 加载配置，drop 协议生效

## 五、总结

### 5.1 关键点

1. **不需要新增 API 接口**
   - drop 协议是标准 outbound 协议
   - 通过配置文件管理，不需要动态 API

2. **只需要前端支持**
   - 添加 drop 协议选项
   - 添加配置表单（丢包率、方向）
   - 更新协议设置类

3. **配置应用方式**
   - 通过生成配置文件并重启 Xray
   - 与现有 outbound 配置方式一致

### 5.2 实现步骤

1. ✅ Xray-core 实现 drop 协议（已完成规划）
2. ⏳ 3x-ui 前端添加 drop 协议支持
3. ⏳ 测试验证

### 5.3 优势

- **无需修改后端 API**：减少开发工作量
- **配置方式统一**：与现有 outbound 配置方式一致
- **易于维护**：配置管理简单清晰
