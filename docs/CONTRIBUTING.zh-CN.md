# 参与贡献 wsh

感谢你考虑为本项目做贡献！英文版请见 [CONTRIBUTING.md](../CONTRIBUTING.md)。

## 开发环境

依赖：**Go 1.27+**、[bun](https://bun.sh)（构建前端）与平台 WebView 运行时：

- **Windows** — Win10/11 自带 WebView2；交叉编译需 mingw-w64
- **Linux (Debian/Ubuntu)** — `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev build-essential pkg-config`
- **macOS** — Xcode Command Line Tools

```bash
./dev.sh    # 前端热更新调试（Vite :5173 + 后端 :17777）
./run.sh    # 完整构建并启动
```

## 提交变更

1. Fork 后从 `main` 拉出功能分支（`feat/my-feature`、`fix/my-bugfix`）
2. 保持与周边代码一致的风格，行为变化请补充或更新测试
3. 提交前跑一遍测试与前端构建：

   ```bash
   go test ./...
   cd frontend && bun install && bun run build
   ```

4. 使用 PR 模板发起 Pull Request，并关联相关 issue

## 规范

- 一个 PR 只做一件事，保持聚焦
- 界面文案必须走 i18n 词典（`frontend/src/lib/i18n/`），不要在组件里硬编码
- Commit message 遵循 [Conventional Commits](https://www.conventionalcommits.org/)
- Go 代码用 `gofmt` 格式化

## 报告问题

请使用 issue 模板；安全漏洞请走 [SECURITY.md](../SECURITY.md)，不要公开提交。

## 许可证

提交即表示同意你的贡献以 [MIT](../LICENSE) 许可证发布。
