# hub

[![Go Reference](https://pkg.go.dev/badge/github.com/jaz303/hub.svg)](https://pkg.go.dev/github.com/jaz303/hub)

`hub` is a multi-user WebSocket hub based on [nhooyr.io/websocket](https://github.com/nhooyr/websocket) intended for use in chat-like/collaborative programs that need to support connections from multiple clients, and handle message/connection events in a straightforward manner.

`hub` provides a serialised view of server activity, exposing this through a simple channel-oriented interface. A basic integration can be achieved with a single loop:

```golang
for {
    select {
    case conn := <-server.Connections():
        // handle new connection
    case conn := <-server.Disconnections():
        // handle new connection
    case msg := <-server.Incoming():
        // handle incoming message
    }
}
```

Behind the scenes `hub` manages all per-connection workflow and state, including authentication, active roster maintenance, incoming message parsing, and outgoing message queueing/encoding.

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

To run:

```shell
go build -o demo/run github.com/jaz303/hub/demo
./demo/run
```

You should now be able to run the demo by accessing `http://localhost:8080` in your browser. To auto-connect, append the `username` query param (e.g. `http://localhost:8080/?username=jason`).

## TODO

  - Rate limiting for outgoing messages?
  - If a receiver can't keep up, is it worth having a policy option to close it, instead of stalling the program?
  - Callback/notification when outgoing message written to socket (buffered channel)
  - Customisable filtering for outgoing message targets

# Copyright

&copy; 2023-2024 Jason Frame
