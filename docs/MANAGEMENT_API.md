# Tunnox Management API 文档

## 概述

Tunnox Management API 提供 HTTP REST API，允许外部系统（如商业平台、管理后台）对 Tunnox 服务器进行管理和监控。

## 特性

- ✅ **用户管理**：创建、查询、更新、删除用户
- ✅ **客户端管理**：创建托管客户端、查询客户端信息、强制下线
- ✅ **端口映射管理**：创建、查询、更新、删除端口映射
- ✅ **统计查询**：用户统计、客户端统计、系统统计
- ✅ **节点管理**：查询节点信息
- ✅ **认证**：支持 API Key 和 JWT 认证
- ✅ **CORS**：跨域资源共享支持
- ✅ **限流**：请求限流保护

## 配置

### 在 server config.yaml 中启用

```yaml
management_api:
  enabled: true
  listen_addr: ":9000"
  
  auth:
    type: "api_key"  # api_key / jwt / none
    secret: "your-secret-key-min-32-chars-long"
  
  cors:
    enabled: true
    allowed_origins:
      - "http://localhost:3000"
      - "https://admin.example.com"
    allowed_methods:
      - GET
      - POST
      - PUT
      - DELETE
    allowed_headers:
      - Authorization
      - Content-Type
  
  rate_limit:
    enabled: true
    requests_per_second: 100
    burst: 200
```

## 认证

### API Key 认证

在请求头中携带 API Key：

```http
GET /api/v1/users/100000001
Authorization: Bearer your-api-key-here
```

### JWT 认证

使用 JWT Token：

```http
GET /api/v1/users/100000001
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

## API 端点

### 1. 健康检查

```http
GET /health
```

**响应**：
```json
{
  "success": true,
  "data": {
    "status": "ok",
    "time": "2025-11-25T10:00:00Z"
  }
}
```

---

### 2. 用户管理

#### 创建用户

```http
POST /api/v1/users
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY

{
  "username": "john_doe",
  "email": "john@example.com"
}
```

**响应 201**：
```json
{
  "success": true,
  "data": {
    "id": "100000001",
    "username": "john_doe",
    "email": "john@example.com",
    "status": "active",
    "created_at": "2025-11-25T10:00:00Z"
  }
}
```

#### 获取用户信息

```http
GET /api/v1/users/{user_id}
Authorization: Bearer YOUR_API_KEY
```

**响应 200**：
```json
{
  "success": true,
  "data": {
    "id": "100000001",
    "username": "john_doe",
    "email": "john@example.com",
    "status": "active"
  }
}
```

#### 更新用户

```http
PUT /api/v1/users/{user_id}
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY

{
  "email": "newemail@example.com",
  "status": "active"
}
```

#### 删除用户

```http
DELETE /api/v1/users/{user_id}
Authorization: Bearer YOUR_API_KEY
```

**响应 204**: No Content

#### 列出用户

```http
GET /api/v1/users?type=registered
Authorization: Bearer YOUR_API_KEY
```

**响应 200**：
```json
{
  "success": true,
  "data": {
    "users": [...],
    "total": 150
  }
}
```

#### 列出用户的客户端

```http
GET /api/v1/users/{user_id}/clients
Authorization: Bearer YOUR_API_KEY
```

#### 列出用户的端口映射

```http
GET /api/v1/users/{user_id}/mappings
Authorization: Bearer YOUR_API_KEY
```

---

### 3. 客户端管理

#### 创建托管客户端

```http
POST /api/v1/clients
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY

{
  "user_id": "100000001",
  "client_name": "My Home Server",
  "client_desc": "Ubuntu 22.04 NAS"
}
```

**响应 201**：
```json
{
  "success": true,
  "data": {
    "id": 601234567,
    "auth_code": "client-abc123def456",
    "user_id": "100000001",
    "name": "My Home Server",
    "type": "managed",
    "status": "offline",
    "created_at": "2025-11-25T10:00:00Z"
  }
}
```

#### 获取客户端信息

```http
GET /api/v1/clients/{client_id}
Authorization: Bearer YOUR_API_KEY
```

#### 更新客户端

```http
PUT /api/v1/clients/{client_id}
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY

{
  "client_name": "Updated Name",
  "status": "offline"
}
```

#### 删除客户端

```http
DELETE /api/v1/clients/{client_id}
Authorization: Bearer YOUR_API_KEY
```

#### 强制下线客户端

```http
POST /api/v1/clients/{client_id}/disconnect
Authorization: Bearer YOUR_API_KEY
```

**响应 200**：
```json
{
  "success": true,
  "data": {
    "message": "Client disconnected successfully"
  }
}
```

#### 认领匿名客户端

```http
POST /api/v1/clients/claim
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY

{
  "anonymous_client_id": 201234567,
  "user_id": "100000001",
  "new_client_name": "Claimed Server"
}
```

**响应 200**：
```json
{
  "success": true,
  "data": {
    "new_client_id": 602345678,
    "new_auth_code": "client-xyz789",
    "message": "Client claimed successfully"
  }
}
```

#### 列出客户端的端口映射

```http
GET /api/v1/clients/{client_id}/mappings
Authorization: Bearer YOUR_API_KEY
```

---

### 4. 端口映射管理

#### 创建端口映射

```http
POST /api/v1/mappings
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY

{
  "user_id": "100000001",
  "source_client_id": 601234567,
  "target_client_id": 602345678,
  "protocol": "tcp",
  "target_host": "localhost",
  "target_port": 3306,
  "local_port": 13306,
  "enable_compression": false,
  "enable_encryption": true
}
```

**响应 201**：
```json
{
  "success": true,
  "data": {
    "id": "mapping-001",
    "status": "active",
    "created_at": "2025-11-25T10:00:00Z"
  }
}
```

#### 获取端口映射信息

```http
GET /api/v1/mappings/{mapping_id}
Authorization: Bearer YOUR_API_KEY
```

#### 更新端口映射

```http
PUT /api/v1/mappings/{mapping_id}
Content-Type: application/json
Authorization: Bearer YOUR_API_KEY

{
  "status": "disabled"
}
```

#### 删除端口映射

```http
DELETE /api/v1/mappings/{mapping_id}
Authorization: Bearer YOUR_API_KEY
```

**响应 204**: No Content

---

### 5. 统计查询

#### 获取用户统计

```http
GET /api/v1/stats/users/{user_id}
Authorization: Bearer YOUR_API_KEY
```

**响应 200**：
```json
{
  "success": true,
  "data": {
    "user_id": "100000001",
    "total_clients": 5,
    "online_clients": 3,
    "total_mappings": 20,
    "active_mappings": 15,
    "current_month_traffic": 10737418240,
    "bandwidth_usage": 1048576
  }
}
```

#### 获取客户端统计

```http
GET /api/v1/stats/clients/{client_id}
Authorization: Bearer YOUR_API_KEY
```

**响应 200**：
```json
{
  "success": true,
  "data": {
    "client_id": 601234567,
    "online_duration": 86400,
    "total_bytes_sent": 1073741824,
    "total_bytes_received": 2147483648,
    "active_mappings": 3
  }
}
```

#### 获取系统统计

```http
GET /api/v1/stats/system
Authorization: Bearer YOUR_API_KEY
```

**响应 200**：
```json
{
  "success": true,
  "data": {
    "total_users": 1000,
    "total_clients": 5000,
    "online_clients": 3000,
    "total_mappings": 20000,
    "active_mappings": 15000,
    "total_bandwidth": 104857600,
    "total_nodes": 5
  }
}
```

---

### 6. 节点管理

#### 获取在线节点列表

```http
GET /api/v1/nodes
Authorization: Bearer YOUR_API_KEY
```

**响应 200**：
```json
{
  "success": true,
  "data": {
    "nodes": [
      {
        "node_id": "node-001",
        "address": "192.168.1.10:8080",
        "online_clients": 500,
        "last_heartbeat": "2025-11-25T10:00:00Z"
      }
    ],
    "total": 5
  }
}
```

#### 获取节点详情

```http
GET /api/v1/nodes/{node_id}
Authorization: Bearer YOUR_API_KEY
```

**响应 200**：
```json
{
  "success": true,
  "data": {
    "node_id": "node-001",
    "address": "192.168.1.10:8080",
    "online_clients": 500,
    "uptime": 86400,
    "version": "v2.2.0"
  }
}
```

---

## 错误响应

所有错误响应遵循统一格式：

```json
{
  "success": false,
  "error": "Error message here"
}
```

### 常见 HTTP 状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 请求成功 |
| 201 | 创建成功 |
| 204 | 删除成功（无内容） |
| 400 | 请求参数错误 |
| 401 | 未认证或认证失败 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

---

## 使用示例

### curl 示例

```bash
# 设置 API Key
API_KEY="your-api-key-here"
API_BASE="http://localhost:9000/api/v1"

# 创建用户
curl -X POST "$API_BASE/users" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "john_doe",
    "email": "john@example.com"
  }'

# 获取用户信息
curl -X GET "$API_BASE/users/100000001" \
  -H "Authorization: Bearer $API_KEY"

# 创建客户端
curl -X POST "$API_BASE/clients" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "100000001",
    "client_name": "My Server"
  }'

# 创建端口映射
curl -X POST "$API_BASE/mappings" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "100000001",
    "source_client_id": 601234567,
    "target_client_id": 602345678,
    "protocol": "tcp",
    "target_host": "localhost",
    "target_port": 3306
  }'

# 获取系统统计
curl -X GET "$API_BASE/stats/system" \
  -H "Authorization: Bearer $API_KEY"
```

### Python 示例

```python
import requests

API_KEY = "your-api-key-here"
API_BASE = "http://localhost:9000/api/v1"

headers = {
    "Authorization": f"Bearer {API_KEY}",
    "Content-Type": "application/json"
}

# 创建用户
response = requests.post(f"{API_BASE}/users", headers=headers, json={
    "username": "john_doe",
    "email": "john@example.com"
})
print(response.json())

# 获取系统统计
response = requests.get(f"{API_BASE}/stats/system", headers=headers)
stats = response.json()
print(f"Total users: {stats['data']['total_users']}")
```

### JavaScript (Node.js) 示例

```javascript
const axios = require('axios');

const API_KEY = 'your-api-key-here';
const API_BASE = 'http://localhost:9000/api/v1';

const headers = {
  'Authorization': `Bearer ${API_KEY}`,
  'Content-Type': 'application/json'
};

// 创建用户
async function createUser() {
  const response = await axios.post(`${API_BASE}/users`, {
    username: 'john_doe',
    email: 'john@example.com'
  }, { headers });
  console.log(response.data);
}

// 获取系统统计
async function getSystemStats() {
  const response = await axios.get(`${API_BASE}/stats/system`, { headers });
  console.log(`Total users: ${response.data.data.total_users}`);
}

createUser();
getSystemStats();
```

---

## 安全建议

1. **生产环境必须使用 HTTPS**
2. **API Key 应至少32字符，使用强随机生成**
3. **不要在客户端代码中硬编码 API Key**
4. **使用环境变量或密钥管理服务存储 API Key**
5. **定期轮换 API Key**
6. **限制 CORS 允许的源，不要使用通配符 `*`**
7. **启用请求限流防止滥用**
8. **定期审计 API 访问日志**

---

## 集成指南

### 与 Web 管理后台集成

```typescript
// frontend/src/services/tunnox-api.ts
import axios from 'axios';

const API_BASE = process.env.REACT_APP_TUNNOX_API_BASE;
const API_KEY = process.env.REACT_APP_TUNNOX_API_KEY;

const client = axios.create({
  baseURL: API_BASE,
  headers: {
    'Authorization': `Bearer ${API_KEY}`,
    'Content-Type': 'application/json'
  }
});

export const tunnoxAPI = {
  // 用户管理
  createUser: (data) => client.post('/users', data),
  getUser: (userId) => client.get(`/users/${userId}`),
  
  // 客户端管理
  createClient: (data) => client.post('/clients', data),
  getClient: (clientId) => client.get(`/clients/${clientId}`),
  
  // 统计
  getSystemStats: () => client.get('/stats/system'),
};
```

---

## 常见问题

### Q: 如何获取 API Key？

A: API Key 在 server 配置文件中设置。生产环境建议使用环境变量或密钥管理服务。

### Q: API 支持分页吗？

A: 当前版本返回所有数据。后续版本将添加分页支持。

### Q: 可以使用自定义认证吗？

A: 可以设置 `auth.type: "none"` 禁用认证，然后在前置网关（如 Nginx）实现自定义认证。

### Q: 如何限制 API 访问权限？

A: 当前版本所有 API 共享一个 API Key。企业版将支持基于角色的访问控制（RBAC）。

---

## 路线图

- [ ] 分页支持
- [ ] WebSocket 实时推送
- [ ] GraphQL 支持
- [ ] RBAC 权限控制
- [ ] OAuth2 集成
- [ ] Webhook 通知
- [ ] API 访问日志审计
- [ ] API 速率限制精细化控制

---

## 支持

如有问题，请提交 Issue 或联系技术支持。

**文档版本**: v2.2  
**最后更新**: 2025-11-25

