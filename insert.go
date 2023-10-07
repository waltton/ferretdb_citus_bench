package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/pkg/errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

var batch = 25
var batches = 20   // how many batches will be inserted per iteration
var iterations = 5 // how many times the whole process will run, setting up a table from scratch and inserting data

func insert(client *mongo.Client, db *sql.DB, dist bool) (err error) {
	_, _, _, dataArrAny, err := loadJsonDataFromDisk()
	if err != nil {
		return err
	}

	dataArrAny = dataArrAny[:batch*batches]

	pIterations := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "iterations",
		Help: "Number of iterations per type",
	})

	pBatchSize := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "batch_size",
		Help: "Size of the batch",
	})

	pFinishedAt := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "finished_at",
		Help: "Time that the run finished at",
	})

	pIterations.Set(float64(iterations))

	pBatchSize.Set(float64(batch))

	p := push.New(pushGateway, "insert")
	p.Collector(pIterations)
	p.Collector(pBatchSize)

	pQuery := prometheus.NewSummary(prometheus.SummaryOpts{
		Name:       "query",
		Help:       "Count of executed queries",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	})

	for i := 0; i < iterations; i++ {
		err = dbinsert(client, i, dataArrAny, &pQuery, db, dist)
		if err != nil {
			return errors.Wrapf(err, "fail on iteration #%d", i)
		}
	}

	p.Collector(pQuery)

	pFinishedAt.SetToCurrentTime()
	p.Collector(pFinishedAt)

	runTS := time.Now().Format("2006-01-02T15:04:05")
	p.Grouping("run_ts", runTS)

	if err := p.Push(); err != nil {
		return errors.Wrap(err, "fail to push metrics to gateway")
	}

	grafanaBaseURL := "http://localhost:3000"
	queryDashboard := "b3a7d255-4083-4a5a-80be-da20613ccd60/postgresql-benchmark-load"

	murl := fmt.Sprintf("%s/d/%s?orgId=1&var-job=insert&var-run_ts=%s", grafanaBaseURL, queryDashboard, runTS)
	fmt.Printf("metrics are available on: %s\n", murl)

	return nil
}

func loadJsonDataFromDisk() (bdata []byte, dataRaw []json.RawMessage, dataParsed []map[string]any, dataArrAny []any, err error) {
	bdata, err = data.ReadFile(recordsPath)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrapf(err, "fail to read records %s", recordsPath)
	}

	err = json.Unmarshal(bdata, &dataRaw)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "fail to decode dataset into slice of raw json")
	}

	err = json.Unmarshal(bdata, &dataParsed)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "fail to decode dataset into slice of maps")
	}

	dataArrAny = make([]any, len(dataParsed))
	for i := range dataParsed {
		dataArrAny[i] = dataParsed[i]
	}

	return bdata, dataRaw, dataParsed, dataArrAny, nil
}

func dbinsert(client *mongo.Client, i int, data []any, pQuery *prometheus.Summary, db *sql.DB, dist bool) (err error) {
	var (
		qc               int
		qd, qdmax, qdmin time.Duration
	)

	coll := client.Database("ferretdb").Collection("test-collection")

	if dist {
		err = DropDistributeTable(db, coll)
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			return
		}
	}

	err = coll.Drop(context.TODO())
	if err != nil {
		return err
	}

	coll = client.Database("ferretdb").Collection("test-collection")

	if dist {
		err = DistributeTable(db, coll)
		if err != nil {
			return
		}
	}

	for i := 0; i < len(data); i += batch {
		fmt.Print(".")

		j := i + batch
		if j > len(data) {
			break
		}

		b := make([]interface{}, len(data[i:j]))
		for k := range data[i:j] {
			b[k] = data[i:j][k]
		}

		begin := time.Now()

		_, err = coll.InsertMany(context.TODO(), data[i:j])
		if err != nil {
			err = errors.Wrapf(err, "fail to insert records from %d:%d", i, j)
			return
		}

		d := time.Since(begin)
		if pQuery != nil {
			(*pQuery).Observe(d.Seconds())
		}

		qd += d

		qc += 1
		if qdmax == 0 && qdmin == 0 {
			qdmax, qdmin = d, d
		}
		if d < qdmin {
			qdmin = d
		}
		if d > qdmax {
			qdmax = d
		}
	}

	fmt.Printf("\n")
	fmt.Printf("count: %v\n", qc)
	fmt.Printf("duration: %v\n", qd)
	fmt.Printf("min: %v, avg: %v, max: %v\n", qdmin, qd/time.Duration(qc), qdmax)
	fmt.Println("")

	return
}

func values(batch int) string {
	rows := []string{}
	for i := 1; i <= batch; i++ {
		rows = append(rows, fmt.Sprintf(("($%d)"), i))
	}

	return strings.Join(rows, ",")
}

func prepTable(db *sql.DB, fieldType string) (tblName string, err error) {
	tblName = fmt.Sprintf("tbl_%s", fieldType)

	tmplDropTable := "TRUNCATE %s"
	_, err = db.Exec(fmt.Sprintf(tmplDropTable, tblName))
	if err != nil {
		return
	}

	return
}
