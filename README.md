# Blocker

**Note: Blocker is still under development.  Check back soon.**

Blocker is a ridiculously simple stateless volume plugin for Docker.  It makes
using [Amazon Elastic Block Store](https://aws.amazon.com/ebs/) volumes trivial,
a pretty useful thing for microservices like databases.

Blocker was designed to work with [Docker](https://docs.docker.com/)
[Machine](https://docs.docker.com/machine/) and
[Swarm](https://docs.docker.com/swarm/) and has a low carbon impact.  It doesn't
do much other than mounting and unmounting volumes on your behalf.

To install Blocker, simply run:

    wget -qO- https://TODO/ | sh

Once installed, you can mount a volume in the usual Docker style, but passing
`--volume-driver blocker` and specifying your EBS volume ID as the name:

    docker run -v vol-933e6c67:/test --volume-driver blocker ...

The target volume must be in the same AWS region and availability zone as the
machine running Docker.  The daemon will automatically attach and detach volumes
as necessary.

