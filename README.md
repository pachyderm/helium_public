This is still under active development, please expect breaking changes.

At a very high level - this is a standardized API around provisioning pachyderm instances - using a variety of backends.

The different backends might be used to point at different clouds etc. (WIP note, backend and runner are still used somewhat interchangably throughout the codebase, switching to backend)


#Running Instructions:


In a terminal tab run:
```shell
HELIUM_MODE=API go run main.go
```

In another terminal tab run:
```shell
curl -X POST -H "X-Pach: NotAGoodSecret" -d '{"version": "1", "backend": "gcp-namespace-pulumi", "spec": {"auth_enabled": false}}'  localhost:2323/api/create
```


# Known Issues:

The tests are broken, but shouldn't affect anything other than controllers
The interface for backend is likely to change slightly, with controller() and register() most likely to shift.
