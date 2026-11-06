# The formula for the devvmhq/homebrew-tap repo. scripts/release.sh fills in @URL@ and @SHA256@ and writes
# the result to dist/devvm.rb. Homebrew reads the version from the URL (devvm-<version>.tar.gz).
class Devvm < Formula
  desc "Isolated Lima VM per project folder, with shared read-only toolchains"
  homepage "https://github.com/devvmhq/devvm"
  url "@URL@"
  sha256 "@SHA256@"
  license "MIT"

  depends_on "go" => :build
  depends_on "lima"

  def install
    ENV["CGO_ENABLED"] = "0"
    ldflags = "-s -w -X github.com/w3cj/devvm/internal/cli.version=#{version}"
    system "go", "build", *std_go_args(ldflags:), "./cmd/devvm"
  end

  test do
    assert_equal version.to_s, shell_output("#{bin}/devvm --version").strip
  end
end
