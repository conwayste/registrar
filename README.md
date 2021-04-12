Registrar for Conwayste Servers
==============================

The registrar maintains a list of all reachable Conwayste servers. Each server
will periodically register itself via HTTP with the registrar. The registrar
periodically sends Conwayste protocol (UDP) `GetStatus` commands to all servers that
have been registered. Any servers that reply with `Status` responses appear in
the list. Conwayste clients retrieve the list to know which servers to connect
to.

## Endpoints

* `GET /servers` - retrieve a list of reachable Conwayste servers.

* `POST /addServer` - register a Conwayste server. The request body should look like this:
```
{
  "host_and_port": "myserver.example.com:2016"
}
```

## Installing and Running

You probably don't need to do this, since there is [an official registrar](https://registry.conwayste.rs/servers), but here are the instructions anyway. Use a recent version of Go (1.15+ or so).

```
git clone https://github.com/conwayste/registrar
cd registrar
go mod download all
go build cmd/registrar.go
./registrar
```

You can run `./registrar -h` to see a list of available flags and their meanings.
