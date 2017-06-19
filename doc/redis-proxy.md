Redis Proxy - usage
===================


Feature overview
----------------

* TLS termination: expose TLS socket, forward requests to non-TLS
  Redis.
* Forward connections to upstream Redis (TLS or non-TLS, both work).
  Every client gets its own, isolated connection to uplink Redis.
* PAUSE operation:
  * suspend execution of any new commands,
  * wait for currently-executing commands to finish.
* RELOAD operation:
  * verify new configuration, reject it in case of errors (this
    includes an attempt to connect to uplink),
  * PAUSE,
  * load new configuration and resume normal operation without
    breaking client connections.
* Enforce a hard limit on all commands, disconnect clients who break
  it.  This allows pause-and-reload to finish in guaranteed time.
* Separate authentication on client and upstream side: you can
  configure the proxy to require clients to authenticate, and the
  proxy can authenticate with upstream Redis.  These passwords are
  independent, and the proxy does NOT forward AUTH commands from the
  client to upstream Redis.
* Keep track of SELECTed database for every client and re-SELECT it
  after replacing upstream Redis.
* HTTP[S] API for reading and controlling Proxy state.


Unsupported by design
---------------------

* PUB/SUB: `redis-proxy` supports only commands that fit the simple
  request-response model.


Configuration file
------------------

JSON.  Example of full file (comments are here for clarity, but are not
supported in actual configuration):

    {
      "uplink": {                   # <- Upstream Redis.  This is where Proxy
        "addr": "localhost:6379",   #   forwards requests.
        "pass": "redis-password",   # <- Optional.  Clients must AUTH if set.
        "tls": true,
        "cacertfile": "cacert.pem"  # <- TLS requires cacertfile.
      },
      "listen": {                   # <- Proxy server.  This is where Proxy
        "addr": "127.0.0.1:7010",   #    clients connect.
        "pass": "client-password",  # <- Optional.  Clients must AUTH if set.
        "tls": true,
        "certfile": "cert.pem",     # <- TLS requires certfile and keyfile.
        "keyfile": "key.pem"
      },
      "admin": {                    # <- Admin UI (http[s]).  This is where
        "addr": "127.0.0.1:7011",   #    you can see and control proxy state.
        "tls": true,
        "certfile": "cert.pem",     # <- TLS requires certfile and keyfile.
        "keyfile": "key.pem"
      },
      "log_messages": false,        # <- Log all traffic to stderr.
      "read_time_limit_ms": 5000    # <- Hard limit on forwarded requests.
    }

The proxy validates config file at startup, and also when told to
reload while running.

At this point the proxy supports changes only to `listen` subtree on
reload.  Changes in any other place will result in the proxy rejecting
the config file.

Use SIGHUP to reload configuration.  It is safe to do without pausing:
it will not terminate any requests, any ongoing requests will


TLS
---

The proxy will validate server if uplink is configured for TLS.  You
must provide the right CA cert to have TLS uplink.

It does not support TLS-level client authentication on any connection.


HTTP[s] API
-----------

Open `admin.addr` to see proxy status.

Proxy exposes these operations via the interface:

    # pause (returns immediately)
    curl http://<admin.addr>/cmd/ -d cmd=pause
    # pause-and-wait (returns after all connections are suspended)
    curl http://<admin.addr>/cmd/ -d cmd=pause-and-wait
    # unpause
    curl http://<admin.addr>/cmd/ -d cmd=unpause
    # reload config (acts like pause + reload + unpause)
    curl http://<admin.addr>/cmd/ -d cmd=reload
