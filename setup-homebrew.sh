#!/bin/bash
set -e

# cfshare Homebrew 设置脚本
# 这个脚本会：
# 1. 提交 cfshare 的更改并推送
# 2. 创建 v1.0.0 tag 并推送（触发 GitHub Actions 构建）
# 3. 在 GitHub 创建 homebrew-tap 仓库
# 4. 等待 Release 完成后更新 formula

CFSHARE_DIR="$(cd "$(dirname "$0")" && pwd)"
TAP_DIR="$HOME/workdir/homebrew-tap"
VERSION="v1.0.0"

echo "=== cfshare Homebrew 发布脚本 ==="
echo ""

# Step 1: 提交 cfshare 更改
echo "[Step 1/5] 提交 cfshare 更改..."
cd "$CFSHARE_DIR"
git add -A
git commit -m "Add version support and GitHub Actions

- Add --version flag
- Add CI workflow for build and lint
- Add Release workflow for multi-platform builds
- Add release scripts for Homebrew

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>" || echo "Nothing to commit"

git push origin main

echo ""
echo "[Step 2/5] 创建并推送 tag $VERSION..."
git tag -a "$VERSION" -m "Release $VERSION" 2>/dev/null || echo "Tag already exists"
git push origin "$VERSION" 2>/dev/null || echo "Tag already pushed"

echo ""
echo "[Step 3/5] 创建 homebrew-tap 仓库..."
echo "请在 GitHub 上创建仓库: homebrew-tap"
echo "  https://github.com/new"
echo "  仓库名: homebrew-tap"
echo "  设为 Public"
echo ""
read -p "创建好后按 Enter 继续..."

# Step 4: 初始化并推送 homebrew-tap
echo ""
echo "[Step 4/5] 初始化 homebrew-tap 仓库..."
cd "$TAP_DIR"
if [ ! -d ".git" ]; then
    git init
    git remote add origin git@github.com:bunnyf/homebrew-tap.git
fi
git add -A
git commit -m "Initial commit: add cfshare formula" || echo "Nothing to commit"
git branch -M main
git push -u origin main

echo ""
echo "[Step 5/5] 等待 GitHub Actions 构建 Release..."
echo ""
echo "请访问以下链接查看构建进度:"
echo "  https://github.com/bunnyf/cfshare/actions"
echo ""
echo "构建完成后（约2-3分钟），运行以下命令更新 formula SHA256:"
echo "  cd $CFSHARE_DIR"
echo "  ./update-homebrew.sh $VERSION"
echo ""
echo "然后提交 homebrew-tap:"
echo "  cd $TAP_DIR"
echo "  git add -A"
echo "  git commit -m 'Update cfshare to $VERSION'"
echo "  git push"
echo ""
echo "完成后用户可以通过以下命令安装:"
echo "  brew tap bunnyf/tap"
echo "  brew install cfshare"
