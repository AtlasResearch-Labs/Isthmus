class Isthmus < Formula
  desc "Cross-Device Zero-Trust Secure Tunnel and Distributed Mesh File System"
  homepage "https://github.com/AtlasResearch-Labs/Isthmus"
  version "0.5.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/AtlasResearch-Labs/Isthmus/releases/latest/download/isthmus-darwin-arm64"
    else
      url "https://github.com/AtlasResearch-Labs/Isthmus/releases/latest/download/isthmus-darwin-amd64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/AtlasResearch-Labs/Isthmus/releases/latest/download/isthmus-linux-arm64"
    else
      url "https://github.com/AtlasResearch-Labs/Isthmus/releases/latest/download/isthmus-linux-amd64"
    end
  end

  def install
    bin.install Dir["isthmus*"].first => "isthmus"
  end

  test do
    system "#{bin}/isthmus", "version"
  end
end
