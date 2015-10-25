#!/bin/sh
set -e

# This script is meant for quick and easy install via:
#     `curl -sSL <url-to-get.sh> | sh`
# or:
#     `wget -qO- <url-to-get.sh> | sh`
#

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
download="https://github.com/joeduffy/blocker/releases/download/v0.1/$filename"

echo "Downloading Blocker $version..."
$sh_c "curl -sSL $download | tar xz"

echo "Installing Blocker..."
$sh_c 'mv blocker /usr/local/bin/blocker'
$sh_c 'chmod +x /usr/local/bin/blocker'
$sh_c 'echo "unix:///var/run/blocker.sock" > /etc/docker/plugins/blocker.sock'
$sh_c 'mv blocker.conf /etc/init/blocker.conf'

echo "Starting the Blocker service..."
$sh_c 'service blocker start'

echo "Done."

