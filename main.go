package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

//go:embed data
var data embed.FS

// https://github.com/algolia/datasets/blob/master/movies/records.json
const recordsPath = "data/records.json"

const pushGateway = "http://localhost:9191/"

var r = rand.New(rand.NewSource(time.Now().Unix()))

func main() {
	if len(os.Args) < 2 {
		log.Print("missing command: insert sizelimit testtoast query")
		return
	}

	db, err := sql.Open("postgres", os.Getenv("PG_DBCONN"))
	if err != nil {
		log.Print(errors.Wrap(err, "fail to connect to db"))
		return
	}

	opts := options.Client().ApplyURI(os.Getenv("MONGO_DBCONN"))
	client, err := mongo.Connect(context.TODO(), opts)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err = client.Disconnect(context.TODO()); err != nil {
			panic(err)
		}
	}()

	switch os.Args[1] {
	case "insert":
		dist := false
		if len(os.Args) == 3 {
			if os.Args[2] == "dist" {
				dist = true
			}
		}

		err := insert(client, db, dist)
		if err != nil {
			log.Print(err)
		}

	default:
		fmt.Printf("%s is an invalid command\n", os.Args[1])
		return
	}
}
