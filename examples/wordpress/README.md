## Wordpress

This example shows running Wordpress and MySQL, similar to the [documented
example](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/example_task_definitions.html),
but replaces the link with an ambassador container.

This allows multiple instances of the Wordpress frontend to be scaled up and
communicate with the same backend database. In addition, if you were to run
mysql replicas with the same family, you could also balance load between them.

## Running this example

To run this example, simply register both task definitions, launch the `mysql`
task, wait for it to finish running, and then launch the `wordpress` task and
browse to it.
