# 发布流程快速参考

## 🚀 三步发布

### 1. 更新版本号和变更日志

```bash
# 编辑 VERSION 文件
echo "1.1.0" > VERSION

# 编辑 CHANGELOG.md，添加新版本的变更说明
vim CHANGELOG.md
```

### 2. 提交并推送

```bash
git add VERSION CHANGELOG.md
git commit -m "chore: bump version to 1.1.0"
git push origin main
```

### 3. 创建标签触发发布

```bash
git tag -a v1.1.0 -m "Release v1.1.0"
git push origin v1.1.0
```

**完成！** GitHub Actions 会自动：
- ✅ 验证版本号
- ✅ 构建多平台二进制
- ✅ 创建 GitHub Release
- ✅ 上传所有文件

## 📝 CHANGELOG.md 模板

```markdown
## [1.1.0] - 2025-01-15

### Added
- 新功能描述

### Changed
- 改进描述

### Fixed
- 修复描述
```

## ⚠️ 注意事项

1. **版本号格式**：必须是 `MAJOR.MINOR.PATCH`（如 `1.1.0`）
2. **标签格式**：必须以 `v` 开头（如 `v1.1.0`）
3. **VERSION 文件**：必须与标签版本一致（不含 `v` 前缀）
4. **CHANGELOG.md**：必须包含对应版本的变更说明

## 🔍 验证发布

发布后检查：

1. GitHub Actions 工作流是否成功
2. GitHub Releases 页面是否有新版本
3. 二进制文件是否已上传
4. 校验和文件是否正确

## 🆘 常见问题

**Q: 版本号验证失败？**  
A: 确保 VERSION 文件只包含版本号（如 `1.1.0`），不要包含 `v` 前缀。

**Q: 如何手动触发发布？**  
A: 在 GitHub 仓库的 Actions 页面，选择 "Release" 工作流，点击 "Run workflow"，输入版本号。

**Q: 如何查看发布状态？**  
A: 在 GitHub 仓库的 Actions 页面查看工作流运行状态。

