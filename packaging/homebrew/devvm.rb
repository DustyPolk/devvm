# The formula for the w3cj/homebrew-tap repo. scripts/release.sh fills in @URL@ and @SHA256@ and writes
# the result to dist/devvm.rb. Homebrew reads the version from the URL (devvm-<version>.tar.gz).
class Devvm < Formula
  desc "Isolated Lima VM per project folder, with shared read-only toolchains"
  homepage "https://github.com/w3cj/devvm"
  url "@URL@"
  sha256 "@SHA256@"
  license "MIT"

  depends_on "go" => :build
  depends_on "lima"

  def install
    ENV["CGO_ENABLED"] = "0"
    ldflags = "-s -w -X github.com/w3cj/devvm/internal/cli.version=#{version}"
    system "go", "build", *std_go_args(ldflags:), "./cmd/devvm"
    # On macOS, the Linux build for this processor too, which Docker VMs' network policy relays and
    # Docker Desktop's builds run in containers: devvm finds it in libexec.
    if OS.mac?
      arch = Hardware::CPU.arm? ? "arm64" : "amd64"
      with_env(GOOS: "linux", GOARCH: arch) do
        system "go", "build", *std_go_args(ldflags:, output: libexec/"devvm-linux-#{arch}"), "./cmd/devvm"
      end
    end
  end

  test do
    assert_equal version.to_s, shell_output("#{bin}/devvm --version").strip
  end
end
