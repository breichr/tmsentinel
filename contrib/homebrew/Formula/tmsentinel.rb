# Homebrew formula template for the tap repository
# (github.com/breichr/homebrew-tmsentinel, Formula/tmsentinel.rb).
#
# The sha256 placeholders are filled in from the release artifacts
# after the release workflow has built them (see README: Distribution).
class Tmsentinel < Formula
  desc "Stateless, FDA-free Time Machine monitoring CLI"
  homepage "https://github.com/breichr/tmsentinel"
  license "MIT"
  version "0.1.0"

  depends_on :macos

  on_arm do
    url "https://github.com/breichr/tmsentinel/releases/download/v0.1.0/tmsentinel-v0.1.0-darwin-arm64.tar.gz"
    sha256 "ARM64_SHA256_PLACEHOLDER"
  end
  on_intel do
    url "https://github.com/breichr/tmsentinel/releases/download/v0.1.0/tmsentinel-v0.1.0-darwin-amd64.tar.gz"
    sha256 "AMD64_SHA256_PLACEHOLDER"
  end

  def install
    bin.install "tmsentinel"
  end

  def caveats
    <<~EOS
      tmsentinel is read-only and FDA-free: it queries the Unified Log
      (log show) and tmutil only. Exit codes are a stable contract
      (0 fresh, 1 internal error, 2 stale, 3 no destination,
      4 destination unreachable).
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/tmsentinel version")
  end
end
