#!/bin/bash
set -e

# 更新 Homebrew formula 脚本
# 用法: ./update-homebrew.sh v1.0.0

VERSION=$1
TAP_DIR="${TAP_DIR:-$HOME/workdir/homebrew-tap}"

if [ -z "$VERSION" ]; then
    echo "用法: ./update-homebrew.sh v1.0.0"
    exit 1
fi

VERSION_NUM=${VERSION#v}
REPO="bunnyf/cfshare"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

echo "=== 更新 Homebrew formula for cfshare $VERSION ==="

# 下载并计算 SHA256
echo "下载 release 文件并计算 SHA256..."

DARWIN_ARM64_SHA=$(curl -sL "${BASE_URL}/cfshare_darwin_arm64.tar.gz" | shasum -a 256 | cut -d' ' -f1)
DARWIN_AMD64_SHA=$(curl -sL "${BASE_URL}/cfshare_darwin_amd64.tar.gz" | shasum -a 256 | cut -d' ' -f1)
LINUX_ARM64_SHA=$(curl -sL "${BASE_URL}/cfshare_linux_arm64.tar.gz" | shasum -a 256 | cut -d' ' -f1)
LINUX_AMD64_SHA=$(curl -sL "${BASE_URL}/cfshare_linux_amd64.tar.gz" | shasum -a 256 | cut -d' ' -f1)

echo "SHA256:"
echo "  darwin_arm64: $DARWIN_ARM64_SHA"
echo "  darwin_amd64: $DARWIN_AMD64_SHA"
echo "  linux_arm64:  $LINUX_ARM64_SHA"
echo "  linux_amd64:  $LINUX_AMD64_SHA"

# 生成 formula
FORMULA="$TAP_DIR/Formula/cfshare.rb"

cat > "$FORMULA" << EOF
class Cfshare < Formula
  desc "Share local files via Cloudflare Tunnel with one command"
  homepage "https://github.com/bunnyf/cfshare"
  version "${VERSION_NUM}"
  license "GPL-3.0-or-later"

  on_macos do
    if Hardware::CPU.arm?
      url "${BASE_URL}/cfshare_darwin_arm64.tar.gz"
      sha256 "${DARWIN_ARM64_SHA}"
    else
      url "${BASE_URL}/cfshare_darwin_amd64.tar.gz"
      sha256 "${DARWIN_AMD64_SHA}"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "${BASE_URL}/cfshare_linux_arm64.tar.gz"
      sha256 "${LINUX_ARM64_SHA}"
    else
      url "${BASE_URL}/cfshare_linux_amd64.tar.gz"
      sha256 "${LINUX_AMD64_SHA}"
    end
  end

  depends_on "cloudflared"

  def install
    if Hardware::CPU.arm?
      if OS.mac?
        bin.install "cfshare_darwin_arm64" => "cfshare"
      else
        bin.install "cfshare_linux_arm64" => "cfshare"
      end
    else
      if OS.mac?
        bin.install "cfshare_darwin_amd64" => "cfshare"
      else
        bin.install "cfshare_linux_amd64" => "cfshare"
      end
    end
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/cfshare --version")
  end
end
EOF

echo ""
echo "✅ Formula 已更新: $FORMULA"
echo ""
echo "下一步:"
echo "  cd $TAP_DIR"
echo "  git add Formula/cfshare.rb"
echo "  git commit -m 'Update cfshare to ${VERSION}'"
echo "  git push"
