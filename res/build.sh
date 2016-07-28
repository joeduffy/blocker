#!/bin/sh
# Makes a new release.
pushd ..
docker run \
    --entrypoint bash \
    --rm -v $(pwd):/go/src/blocker \
    golang \
    -c 'cd /go/src/blocker && \
        go get && \
        go build && \
        mv blocker res/ && \
        cd res && \
        tar czf blocker.$(cat LATEST).$(uname -s)-$(uname -m).tar.gz blocker blocker.service'
popd

