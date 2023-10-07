# ferretdb_citus_bench

Based on my project [pg_json_bench](https://github.com/waltton/pg_json_bench).

This project is to help setting up and comparing the performance of FerretDB on top of a regular PostgreSQL instance vs a Citus cluster.

The approach taken here is "very experimental" (completly hacked).

## Running
**Spin up docker compose with monitoring stack**
```shell
docker compose up -d
```

**Prepare the data**
```shell
mkdir ./data
wget https://raw.githubusercontent.com/algolia/datasets/master/movies/records.json -O ./data/records.json
```


**Build**
```shell
go build -o ferretdb_citus_bench
```

**Run Benchmark**
Example:
```shell
export MONGO_DBCONN="mongodb://postgres:password@localhost:27017/postgres?authMechanism=PLAIN"
export PG_DBCONN="host=localhost port=5432 user=postgres dbname=postgres sslmode=disable"

./ferretdb_citus_bench insert       # for testing insert on regular PostgreSQL instance
./ferretdb_citus_bench insert dist  # for testing insert on Citus
```

On the output the link for the metrics will be displayed, maybe you need to refresh.
Grafana user and password is `admin`
