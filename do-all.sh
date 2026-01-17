#!/bin/bash
set -e
cd "$(dirname "$0")"

echo "=== cfshare + Homebrew 完整发布 ==="
echo ""

# 检查 gh 是否安装
if ! command -v gh &>/dev/null; then
    echo "安装 GitHub CLI..."
    brew install gh
fi

# 检查 gh 是否登录
if ! gh auth status &>/dev/null; then
    echo "请先登录 GitHub CLI:"
    gh auth login
fi

echo "[1/6] 提交 cfshare 更改..."
git add -A
git commit -m "Add version support and GitHub Actions

- Add --version flag
- Add CI workflow for build and lint
- Add Release workflow for multi-platform builds
- Add Homebrew release scripts

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>" 2>/dev/null || echo "Already committed"
git push origin main

echo "[2/6] 创建 v1.0.0 tag..."
git tag -a v1.0.0 -m "Release v1.0.0" 2>/dev/null || echo "Tag exists"
git push origin v1.0.0 2>/dev/null || echo "Tag pushed"

echo "[3/6] 创建 homebrew-tap 仓库..."
gh repo create homebrew-tap --public --description "Homebrew tap for bunnyf" 2>/dev/null || echo "Repo exists"

echo "[4/6] 等待 GitHub Actions 构建..."
echo "查看进度: https://github.com/bunnyf/cfshare/actions"
echo ""
echo "等待 Release 完成 (最多3分钟)..."

for i in {1..36}; do
    if curl -s "https://api.github.com/repos/bunnyf/cfshare/releases/tags/v1.0.0" | grep -q "cfshare_darwin_arm64"; then
        echo "✅ Release 已完成!"
        break
    fi
    echo -n "."
    sleep 5
done
echo ""

echo "[5/6] 更新 Homebrew formula..."
TAP_DIR="$HOME/workdir/homebrew-tap"
./update-homebrew.sh v1.0.0

echo "[6/6] 推送 homebrew-tap..."
cd "$TAP_DIR"
if [ ! -d ".git" ]; then
    git init
    git remote add origin git@github.com:bunnyf/homebrew-tap.git
fi
git add -A
git commit -m "Add cfshare formula v1.0.0" 2>/dev/null || echo "Already committed"
git branch -M main
git push -u origin main 2>/dev/null || git push origin main

echo ""
echo "============================================"
echo "🎉 完成!"
echo "============================================"
echo ""
echo "安装测试:"
echo "  brew tap bunnyf/tap"
echo "  brew install cfshare"
echo "  cfshare --version"
