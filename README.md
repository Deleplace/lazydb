# Lazy DB connection strategy for a multitenant app

Sample Go code of a server connecting to a database on-demand, depending on the incoming request.

The first time a request needs to access the database of a tenant T, the server initiation the new DB connection. After that,
subsequent requests for the same tenant T should reuse the connection, rather than creating a new one.
