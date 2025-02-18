# SQLite Admin

[![Build](https://github.com/joelseq/sqliteadmin-go/actions/workflows/build.yml/badge.svg)](https://github.com/joelseq/sqliteadmin-go/actions/workflows/build.yml)

SQLite Admin provides a lightweight web server to interact with a SQLite database. It allows you to:

- Browse tables and their schemas.
- View table data along with adding filters, limits and offsets.
- Modify individual columns in existing rows.

It can either by installed as a binary or embedded into an existing Golang backend as a library.

The web server can be interacted with by going to https://sqliteadmin.dev.

The source code for the web UI can be found at https://github.com/joelseq/sqliteadmin-ui


## Motivation

SQLite is very easy to add as an embedded database but it's difficult to manage the database once it's deployed in an application.

Existing tools primarily focus on local SQLite files, requiring manual interaction through CLI tools or desktop applications. If your SQLite database is running embedded within an application, there are few (if any) solutions that let you inspect, query, and modify it without complex workarounds.

The alternative is to use a cloud-hosted version like those provided by [Turso](https://turso.tech/) which enables interacting with the database using tools like [Drizzle Studio](https://orm.drizzle.team/drizzle-studio/overview). This adds complexity to the setup and deployment of your application and you lose out on the value of having an embedded database.

This project fills that gap by providing an easy way to view and manage an embedded SQLite database via a web UI—no need to migrate to a cloud provider or use ad-hoc solutions.

## Installing as a binary

1. Using `go install`:

```bash
go install github.com/joelseq/sqliteadmin-go/cmd/sqliteadmin@latest
```

2. Using `go build` (after cloning the repository):

```bash
make build
```
This will add the `sqliteadmin` binary to `/tmp/bin`

### Usage

In order to add authentication, the following environment variables are required: `SQLITEADMIN_USERNAME`, `SQLITEADMIN_PASSWORD`.

e.g.:
```bash
export SQLITEADMIN_USERNAME=user
export SQLITEADMIN_PASSWORD=password
```

Start the server

```bash
sqliteadmin serve <path to sqlite db> -p 8080
```

Your SQLite database can now be accessed by visiting https://sqliteadmin.dev and providing the credentials and endpoint (including port).

## Using as a library

Check out the `examples/` directory to see how to integrate it into an existing Golang backend.

You can also run the examples to test out the admin UI:

```bash
go run examples/chi/main.go
```

This will spin up a server on `http://localhost:8080`. You can then interact with your local server by going to `https://sqliteadmin.dev` and passing in the following credentials:

```
username: user
password: password
endpoint: http://localhost:8080/admin
```

> [!NOTE]  
> If you are seeing "An unexpected error occurred" when trying to connect, try disabling your adblock.

## Inspiration

The UI is heavily inspired by [Drizzle Studio](https://orm.drizzle.team/drizzle-studio/overview).
