#!/bin/bash
set -e
cd "$(dirname "$0")"

echo "=== cfshare Homebrew 发布 ==="

# Step 1: 提交 cfshare 更改
echo "[1/3] 提交 cfshare 更改并推送..."
git add -A
git commit -m "Add version support and GitHub Actions

- Add --version flag
- Add CI workflow for build and lint
- Add Release workflow for multi-platform builds
- Add Homebrew release scripts

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>" || true
git push origin main

# Step 2: 创建 tag
echo "[2/3] 创建 v1.0.0 tag..."
git tag -a v1.0.0 -m "Release v1.0.0" 2>/dev/null || echo "Tag exists"
git push origin v1.0.0 2>/dev/null || echo "Tag pushed"

echo ""
echo "✅ cfshare 更新已推送！"
echo ""
echo "=== 下一步 ==="
echo ""
echo "1. 等待 GitHub Actions 构建完成 (约2分钟):"
echo "   https://github.com/bunnyf/cfshare/actions"
echo ""
echo "2. 在 GitHub 创建新仓库 'homebrew-tap':"
echo "   https://github.com/new"
echo ""
echo "3. 构建完成后运行:"
echo "   cd ~/workdir/homebrew-tap"
echo "   git init"
echo "   git remote add origin git@github.com:bunnyf/homebrew-tap.git"
echo "   cd ~/workdir/cfshare"
echo "   ./update-homebrew.sh v1.0.0"
echo "   cd ~/workdir/homebrew-tap"
echo "   git add -A && git commit -m 'Add cfshare formula' && git push -u origin main"
echo ""
echo "4. 安装测试:"
echo "   brew tap bunnyf/tap"
echo "   brew install cfshare"
