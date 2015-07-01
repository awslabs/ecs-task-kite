# ECS Task Kite

## What is "ECS Task Kite"

You and your space-buddy are stowing away on a starship in a pair of corrugated
tin containers. You know your best gal, Monday, is stowed away aboard another
spaceship near the station. "Hey buddy, did you bring your Kite?" you knock on
the container wall to get his attention, "I really need to flash a sig at my
girl". "Yeah, my Kite's already out on the Sail, just give me her deets".
Moments later, without Monday having a clue where you are, your buddy has used
the reflected radiation (or lack thereof) on his Kite to find her and get your
signal through. All would be well&hellip; if there weren't five girls named
Monday and your buddy's Kite picked the wrong one at random.

Fortunately for you, the "Monday Quintuplets" are well designed and stateless
so you can hardly tell the difference betwixt the lot of them.

## No really, what is it?

ECS Task Kite is a proof of concept [ambassador
container](https://docs.docker.com/articles/ambassador_pattern_linking/) using
the ECS APIs.  It's meant to be tiny in terms of memory and disk footprint and
simple in terms of operation.

Unlike the ambassador pattern documented above, it is expected that this
ambassador only be run on the consumer. It is also expected that the server is
not identified by IP directly, but rather by ECS Task Family or by ECS Service
Name. Furthermore, it is capable of proxying between multiple servers that meet
the above criterion (randomly currently).

It is also expected that it be used via either container links or sharing the
network namespace with the consumer. In the case of linking, it does not
`EXPOSE` the appropriate ports (due to them being dynamically discovered at
runtime), and thus will not work with the `--icc` option disabled. This option
is enabled by default however.

## Usage

To use the Task Kite, simply add it to your task definition, link to it, and
set the below options as appropriate.

Required:
 * Flag: `-name=<containerName>` set to the name of the container to proxy to within the referenced task or service.
 * Flag: `-family=<taskFamily[:revision]>` XOR `-service=<serviceName>`.

Optional:
 * Flag: `-public=<true|false>`: Whether to proxy to the public IP of the EC2 instance(s); default false.
 * Flag: `-cluster=<cluster>`: The ECS cluster containing the above tasks or service; default "default".
 * Environment variable: `AWS_REGION=<AWS Region>` set to the region of the task(s) you will be ambassadoring; default current region.

The Task Kite will proxy to a task of the specified family or within the
specified service at random when a connection is made to it on a valid port.

## What is it not?

* Production ready
* Highly configurable (perhaps you want HAProxy?)
* Usable outside of ECS (Have I mentioned HAProxy?)
* Well tested

## Examples

See the subdirectories of the `examples` directory.

## License

Apache 2.0

## Contributions

Yes please!
