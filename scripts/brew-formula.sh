#!/bin/sh
# Prints the Homebrew formula for a release. Usage: brew-formula.sh VERSION SHA256SUMS
set -eu
version=$1 sums=$2
sum() { awk -v f="syscall_${version}_$1.tar.gz" '$2 == f { print $1 }' "$sums"; }
url="https://github.com/guitmz/syscall/releases/download/$version/syscall_${version}"
cat <<RUBY
class Syscall < Formula
  desc "Explore Linux system calls from your terminal"
  homepage "https://github.com/guitmz/syscall"
  version "${version#v}"
  license "MIT"

  on_macos do
    on_arm do
      url "${url}_darwin_arm64.tar.gz"
      sha256 "$(sum darwin_arm64)"
    end
    on_intel do
      url "${url}_darwin_amd64.tar.gz"
      sha256 "$(sum darwin_amd64)"
    end
  end

  on_linux do
    on_arm do
      url "${url}_linux_arm64.tar.gz"
      sha256 "$(sum linux_arm64)"
    end
    on_intel do
      url "${url}_linux_amd64.tar.gz"
      sha256 "$(sum linux_amd64)"
    end
  end

  def install
    bin.install "syscall"
    generate_completions_from_executable(bin/"syscall", "completion")
  end

  test do
    assert_match "memfd_create", shell_output("#{bin}/syscall memfd_create --color never")
  end
end
RUBY
