# Homebrew formula template for the tap repository
# (github.com/breichr/homebrew-tmsentinel, Formula/tmsentinel.rb).
#
# Release binaries are Developer ID signed and notarized; see
# scripts/release.sh and scripts/notarize.sh in the main repository.
class Tmsentinel < Formula
  desc "Stateless, FDA-free Time Machine monitoring CLI"
  homepage "https://github.com/breichr/tmsentinel"
  license "MIT"
  version "0.1.0"

  depends_on :macos

  on_arm do
    url "https://github.com/breichr/tmsentinel/releases/download/v0.1.0/tmsentinel_v0.1.0_darwin_arm64.zip"
    sha256 "d596f3b3651ea9cc00a3280b7dffc5ea92a9e8d389a042411e3f1526e1df1a12"
  end
  on_intel do
    url "https://github.com/breichr/tmsentinel/releases/download/v0.1.0/tmsentinel_v0.1.0_darwin_amd64.zip"
    sha256 "905055238c395a6c0462adacbd4265b529f5df4628ea3d34ba93d642704546c5"
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
