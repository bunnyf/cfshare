#!/bin/bash
set -e

# cfshare 发布脚本
# 用法: ./release.sh v1.0.0

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "用法: ./release.sh v1.0.0"
    exit 1
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "错误: 版本号格式应为 vX.Y.Z (如 v1.0.0)"
    exit 1
fi

echo "=== 发布 cfshare $VERSION ==="

# 确保工作目录干净
if [ -n "$(git status --porcelain)" ]; then
    echo "错误: 工作目录有未提交的更改"
    echo "请先提交或 stash 更改"
    exit 1
fi

# 创建 tag
echo "创建 tag $VERSION..."
git tag -a "$VERSION" -m "Release $VERSION"

# 推送 tag
echo "推送 tag..."
git push origin "$VERSION"

echo ""
echo "✅ 完成！"
echo ""
echo "GitHub Actions 将自动构建并创建 Release。"
echo "查看进度: https://github.com/bunnyf/cfshare/actions"
echo ""
echo "Release 创建后，运行以下命令更新 Homebrew formula:"
echo "  ./update-homebrew.sh $VERSION"
