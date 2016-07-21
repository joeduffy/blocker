#!/bin/sh
set -e

# This script is meant for quick and easy install via:
#     `curl -sSL <url-to-get.sh> | sh`
# or:
#     `wget -qO- <url-to-get.sh> | sh`
#

# Wrap in () to prevent execution if connection is interrupted.
(
    # Check for valid operating systems and machines.
    if [ "`uname -s -p`" != "Linux x86_64" ]; then
        echo "error: Unsupported operating system '`uname -s -p`';"
        echo "       Only 'Linux x86_64' supported at the moment."
        #exit 1
    fi

    # make sure to sudo as necessary.
    sh_c='sh -c'
    if [ "$user" != 'root' ]; then
        sh_c='sudo -E sh -c'
    fi

    version=`curl -sSL https://raw.githubusercontent.com/joeduffy/blocker/master/res/LATEST`
    filename="blocker.$version.Linux-x86_64.tar.gz"
    download="https://github.com/joeduffy/blocker/releases/download/$version/$filename"

    echo "Downloading Blocker $version..."
    $sh_c "curl -sSL $download | tar xz"

    echo "Installing Blocker..."
    $sh_c 'mv blocker /usr/local/bin/blocker'
    $sh_c 'chmod +x /usr/local/bin/blocker'
    $sh_c 'mkdir -p /etc/blocker/.aws'
    $sh_c 'mkdir -p /etc/docker/plugins'
    $sh_c 'echo "unix:///var/run/blocker.sock" > /etc/docker/plugins/blocker.spec'
    $sh_c 'mv blocker.service /etc/systemd/system/blocker.service'

    echo "Starting the Blocker service..."
    $sh_c 'systemctl enable /etc/systemd/system/blocker.service'
    $sh_c 'systemctl start blocker.service'

    echo "Done."
)
