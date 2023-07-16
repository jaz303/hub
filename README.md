# hub

`hub` is a multi-user WebSocket hub based on [nhooyr.io/websocket](https://github.com/nhooyr/websocket).

`hub` features:

  - channel-based notification of connections, disconnections, and received messages
  - flexible message send operations capable of targetting both logical clients and physical connections
  - pluggable authentication
  - pluggable policies for controlling multiple connections from the same client ID
  - `net/http` compatible interface
  - full `context.Context` support

## Status

Very much alpha - I'm building `hub` in parallel with a couple of my own projects, robustness will improve
over time.

## Example

A fully commented chat example can be found at [demo/main.go](demo/main.go).

## TODO

  - Ping/pong handlers
  - Rate limiting for outgoing messages?
  - If a receiver can't keep up, is it worth having a policy option to close it, instead of stalling the program?
  - Callback/notification when outgoing message written to socket
